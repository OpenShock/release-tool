package release

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
)

// CheckState is the outcome of a pull request change-file check.
type CheckState string

const (
	StateOK      CheckState = "ok"
	StateMissing CheckState = "missing"
	StateInvalid CheckState = "invalid"
	StateSkip    CheckState = "skip"
)

// checkMarker is an invisible HTML comment so the comment stage can recognise
// and update its own sticky comment rather than posting duplicates.
const checkMarker = "<!-- release-tool-check -->"

// Verdict is the result written for the comment stage to consume.
type Verdict struct {
	State CheckState `json:"state"`
	PR    int        `json:"pr"`
	Body  string     `json:"body"`
}

type CheckParams struct {
	Root       string
	BaseBranch string          // PR base branch, looked up in Config.Branches
	Against    string          // git ref to diff against for added change files
	PR         int             // PR number, passed through to the verdict
	Config     *changes.Config // repo config (for Branches)
}

// RunCheck evaluates the change files a pull request adds relative to its base.
// It returns skip when the base is not a configured release branch, missing
// when the PR adds no change file, invalid when an added file fails validation,
// and ok otherwise. It performs no network or comment side effects.
func RunCheck(p CheckParams) (Verdict, error) {
	if p.Config == nil {
		return Verdict{State: StateSkip, PR: p.PR}, nil
	}
	if _, ok := p.Config.Branches[p.BaseBranch]; !ok {
		return Verdict{State: StateSkip, PR: p.PR}, nil
	}

	added, err := git.ChangedChangeFilesSinceRef(p.Root, p.Against)
	if err != nil {
		return Verdict{}, err
	}

	state, detail := StateOK, ""
	switch {
	case len(added) == 0:
		state = StateMissing
	default:
		if _, vErr := changes.ReadSubset(p.Root, added); vErr != nil {
			state, detail = StateInvalid, vErr.Error()
		}
	}

	return Verdict{
		State: state,
		PR:    p.PR,
		Body:  renderCheckBody(state, len(added), detail),
	}, nil
}

func renderCheckBody(state CheckState, count int, detail string) string {
	var b strings.Builder
	b.WriteString(checkMarker)
	b.WriteString("\n### Change file check\n\n")
	switch state {
	case StateOK:
		fmt.Fprintf(&b, "✅ **OK** — %d valid change file(s) added in this PR.\n", count)
	case StateMissing:
		b.WriteString("⚠️ **Missing** — this PR does not add a change file under `.changes/`.\n\n")
		b.WriteString("If this change should show up in the release notes, add one with `release-tool new \"<title>\" --type <major|minor|patch>`.\n")
	case StateInvalid:
		b.WriteString("❌ **Invalid format** — an added change file failed validation:\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n", strings.TrimSpace(detail))
	}
	return b.String()
}

// WriteVerdict writes v as JSON for the comment stage to read from an artifact.
func WriteVerdict(path string, v Verdict) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

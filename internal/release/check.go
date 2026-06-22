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
		ch, vErr := changes.ReadSubset(p.Root, added)
		if vErr != nil {
			state, detail = StateInvalid, vErr.Error()
		} else {
			for _, c := range ch {
				if c.PR != nil || c.PRExplicitNone {
					state = StateInvalid
					detail += fmt.Sprintf("  - %s: pr field must not be set in a PR (it is assigned automatically at release time)\n", c.Filename)
				}
			}
			if state == StateInvalid {
				detail = "invalid change files:\n" + detail
			}
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
		fmt.Fprintf(&b, "If this change should show up in the release notes, add one with `release-tool new \"<title>\" --kind <%s>`.\n", changes.KindList())
	case StateInvalid:
		b.WriteString("❌ **Invalid format** — an added change file failed validation:\n\n")
		// detail embeds attacker-controlled change-file content (filenames, YAML
		// values) and is posted verbatim into a privileged PR comment. Use a
		// fence longer than any backtick run inside it so the content cannot
		// break out of the code block and inject markdown.
		body := strings.TrimSpace(detail)
		fence := safeFence(body)
		fmt.Fprintf(&b, "%s\n%s\n%s\n", fence, body, fence)
	}
	return b.String()
}

// safeFence returns a run of backticks at least one longer than the longest
// backtick run in content (minimum three), so content cannot terminate it.
func safeFence(content string) string {
	longest, cur := 0, 0
	for _, r := range content {
		if r == '`' {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	n := longest + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
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

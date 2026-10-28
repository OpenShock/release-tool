package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
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
	// Findings are the per-file errors and warnings, for CI annotations.
	Findings []changes.Finding `json:"findings,omitempty"`
	// Impact is how the PR moves the next release version, when it adds
	// valid change files.
	Impact *VersionImpact `json:"impact,omitempty"`
}

// VersionImpact describes how a pull request's change files move the next
// release version.
type VersionImpact struct {
	// Bump is the highest bump among the PR's own change files.
	Bump string `json:"bump"`
	// Latest is the latest stable version ("" before the first release).
	Latest string `json:"latest,omitempty"`
	// Before is the next version from the pending changes without this PR
	// ("" when nothing else is pending); After includes the PR.
	Before string `json:"before,omitempty"`
	After  string `json:"after"`
}

// versionImpact computes the PR's effect on the next version from every
// pending change file at HEAD. It returns nil when that cannot be determined
// (another pending file is invalid, or the tags are unreadable), so the check
// itself never fails on it.
func versionImpact(root, tagPrefix string, prChanges []*changes.Change) *VersionImpact {
	all, err := changes.Read(root)
	if err != nil {
		return nil
	}
	latest, err := git.LatestStableTag(root, tagPrefix)
	if err != nil {
		return nil
	}
	after, err := ComputeNext(all, latest)
	if err != nil {
		return nil
	}
	inPR := make(map[string]bool, len(prChanges))
	for _, c := range prChanges {
		inPR[c.Filename] = true
	}
	var others []*changes.Change
	for _, c := range all {
		if !inPR[c.Filename] {
			others = append(others, c)
		}
	}
	impact := &VersionImpact{Bump: HighestBump(prChanges), Latest: latest, After: after}
	if len(others) > 0 {
		if impact.Before, err = ComputeNext(others, latest); err != nil {
			return nil
		}
	}
	return impact
}

type CheckParams struct {
	Root       string
	BaseBranch string          // PR base branch, looked up in Config.Branches
	Against    string          // git ref to diff against for added change files
	PR         int             // PR number, passed through to the verdict
	Config     *changes.Config // repo config (for Branches and IgnorePaths)
}

// RunCheck evaluates the change files a pull request adds relative to its base.
// It returns skip when the base is not a configured release branch or the PR
// only touches ignored paths, missing when the PR adds no change file, invalid
// when an added file fails validation or lint, and ok otherwise. It performs no
// network or comment side effects.
func RunCheck(p CheckParams) (Verdict, error) {
	if p.Config == nil {
		return Verdict{State: StateSkip, PR: p.PR}, nil
	}
	if _, ok := p.Config.Branches[p.BaseBranch]; !ok {
		return Verdict{State: StateSkip, PR: p.PR}, nil
	}

	// Resolve the ref to diff added change files against. On a pull_request
	// merge-ref checkout, HEAD's first parent is the current base tip; prefer it
	// when the caller's base (the event's base.sha) merely trails it, so a stale
	// base on a long-lived or reopened PR doesn't make the diff pick up change
	// files the base branch accrued since the PR was opened.
	against := p.Against
	if baseParent, ok := git.MergeRefBaseParent(p.Root); ok {
		if against == "" || git.IsAncestor(p.Root, against, baseParent) {
			against = baseParent
		}
	}

	added, err := git.ChangedChangeFilesSinceRef(p.Root, against)
	if err != nil {
		return Verdict{}, err
	}

	if len(added) == 0 && len(p.Config.IgnorePaths) > 0 && against != "" {
		touched, err := git.ChangedFilesSinceRef(p.Root, against)
		if err != nil {
			return Verdict{}, err
		}
		if allIgnored(touched, p.Config.IgnorePaths) {
			return Verdict{State: StateSkip, PR: p.PR}, nil
		}
	}

	state := StateOK
	var findings []changes.Finding
	var impact *VersionImpact
	if len(added) == 0 {
		state = StateMissing
	} else {
		ch, vErr := changes.ReadSubset(p.Root, added)
		var invalid *changes.InvalidFilesError
		switch {
		case errors.As(vErr, &invalid):
			findings = invalid.Findings
		case vErr != nil:
			return Verdict{}, vErr
		default:
			for _, c := range ch {
				if c.PR != nil || c.PRExplicitNone {
					findings = append(findings, changes.Finding{File: c.Filename, Severity: changes.SeverityError,
						Message: "pr field must not be set in a PR (it is assigned automatically at release time)"})
				}
			}
			findings = append(findings, changes.Lint(ch)...)
			if len(ch) > 0 {
				impact = versionImpact(p.Root, p.Config.TagPrefix, ch)
			}
		}
		if changes.HasErrors(findings) {
			state = StateInvalid
		}
	}

	return Verdict{
		State:    state,
		PR:       p.PR,
		Body:     renderCheckBody(state, len(added), findings, impact),
		Findings: findings,
		Impact:   impact,
	}, nil
}

// globRe converts a path glob to a regexp: `**` matches across directories,
// `*` and `?` stay within one path segment.
func globRe(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// allIgnored reports whether every file matches one of the globs. An empty
// file list is not "all ignored": a PR with no changes still needs a decision.
func allIgnored(files, globs []string) bool {
	if len(files) == 0 {
		return false
	}
	res := make([]*regexp.Regexp, len(globs))
	for i, g := range globs {
		res[i] = globRe(g)
	}
	for _, f := range files {
		matched := false
		for _, re := range res {
			if re.MatchString(f) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// bumpMeaning explains a bump level in the PR comment.
var bumpMeaning = map[string]string{
	"major": "breaking change",
	"minor": "new or changed behaviour",
	"patch": "fix",
}

// writeImpact renders the version line of the PR comment.
func writeImpact(b *strings.Builder, im *VersionImpact) {
	fmt.Fprintf(b, "\n**Version impact:** **%s** bump (%s). ", im.Bump, bumpMeaning[im.Bump])
	switch {
	case im.Before == "":
		fmt.Fprintf(b, "The next release will be **%s**", im.After)
	case im.Before == im.After:
		fmt.Fprintf(b, "The next release stays **%s**", im.After)
	default:
		fmt.Fprintf(b, "The next release becomes **%s**, up from %s without this PR", im.After, im.Before)
	}
	if im.Latest != "" {
		fmt.Fprintf(b, " (latest stable: %s).\n", im.Latest)
	} else {
		b.WriteString(" (first release).\n")
	}
}

func renderCheckBody(state CheckState, count int, findings []changes.Finding, impact *VersionImpact) string {
	var b strings.Builder
	b.WriteString(checkMarker)
	b.WriteString("\n### Change file check\n\n")
	switch state {
	case StateOK:
		fmt.Fprintf(&b, "✅ **OK**: %d valid change file(s) added in this PR.\n", count)
		if impact != nil {
			writeImpact(&b, impact)
		}
	case StateMissing:
		b.WriteString("⚠️ **Missing**: this PR does not add a change file under `.changes/`.\n\n")
		fmt.Fprintf(&b, "If this change should show up in the release notes, add one with `release-tool new \"<title>\" --kind <%s>`. Otherwise add the `no-changelog` label.\n", changes.KindList())
	case StateInvalid:
		b.WriteString("❌ **Invalid**: an added change file failed validation:\n\n")
	}
	if len(findings) == 0 {
		return b.String()
	}
	if state != StateInvalid {
		b.WriteString("\nSuggestions:\n\n")
	}
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = f.String()
	}
	// The findings embed attacker-controlled change-file content (filenames,
	// YAML values) and are posted verbatim into a privileged PR comment. Use a
	// fence longer than any backtick run inside so the content cannot break out
	// of the code block and inject markdown.
	body := strings.Join(lines, "\n")
	fence := safeFence(body)
	fmt.Fprintf(&b, "%s\n%s\n%s\n", fence, body, fence)
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

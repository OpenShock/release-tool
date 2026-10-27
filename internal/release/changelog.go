package release

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var kindOrder = []string{"added", "changed", "deprecated", "removed", "fixed", "security", "safety", "chore"}

var kindHeading = map[string]string{
	"added":      "Added",
	"changed":    "Changed",
	"deprecated": "Deprecated",
	"removed":    "Removed",
	"fixed":      "Fixed",
	"security":   "Security",
	"safety":     "Safety",
	"chore":      "Chores",
}

// isUserFacing returns true for kinds that appear in the GitHub Release body.
// Chores are developer-internal and excluded from user-facing outputs.
func isUserFacing(kind string) bool { return kind != "chore" }

// issueRef matches a bare "#123" that starts a line or follows whitespace or
// opening punctuation, so "owner/repo#1", "[#1](...)" and "`#1`" are left alone.
var issueRef = regexp.MustCompile(`(^|[\s(,;:])#(\d+)\b`)

// linkRefs turns bare "#123" references into markdown links when the
// repository is known ("owner/repo"); otherwise it returns text unchanged.
// /issues/ is used because GitHub redirects it to the PR when the number is one.
func linkRefs(text, repo string) string {
	if repo == "" {
		return text
	}
	return issueRef.ReplaceAllString(text, "${1}[#${2}](https://github.com/"+repo+"/issues/${2})")
}

// prRef renders the " (#123)" suffix for an entry, linked when repo is known.
func prRef(pr int, repo string) string {
	if repo == "" {
		return fmt.Sprintf(" (#%d)", pr)
	}
	return fmt.Sprintf(" ([#%d](https://github.com/%s/pull/%d))", pr, repo, pr)
}

// listItem matches markdown list lines ("- x", "* x", "+ x", "1. x").
var listItem = regexp.MustCompile(`^([-*+]|\d+[.)])\s+`)

// repoSlug returns "owner/repo" for data's repository, or "" when unknown.
func repoSlug(r *Repository) string {
	if r == nil || r.Owner == "" || r.Repo == "" {
		return ""
	}
	return r.Owner + "/" + r.Repo
}

// renderSections writes KaC-style ### Kind sections for the given kinds filter.
// Pass nil to include all kinds. When withReleaseNote is set, the authored
// `## Release Note` description lines are rendered under each entry (used for
// the GitHub Release body; the CHANGELOG intentionally shows the title only).
// List items in the description are left out there: they tend to be
// implementation detail and make the release body hard to scan. release.json
// still carries them. repo ("owner/repo", may be empty) enables links.
func renderSections(b *strings.Builder, changes []ChangeEntry, withPR bool, filter func(string) bool, withReleaseNote bool, repo string) {
	for _, kind := range kindOrder {
		if filter != nil && !filter(kind) {
			continue
		}
		var entries []ChangeEntry
		for _, c := range changes {
			if c.Kind == kind {
				entries = append(entries, c)
			}
		}
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(b, "\n### %s\n", kindHeading[kind])
		for _, e := range entries {
			badge := ""
			if e.Breaking {
				badge = " **BREAKING**"
			}
			pr := ""
			if withPR && e.PR != nil {
				pr = prRef(*e.PR, repo)
			}
			fmt.Fprintf(b, "- %s%s%s\n", linkRefs(e.Title, repo), badge, pr)
			if withReleaseNote && e.ReleaseNote != nil {
				for _, line := range e.ReleaseNote.Description {
					if listItem.MatchString(line) {
						continue
					}
					fmt.Fprintf(b, "  %s\n", linkRefs(line, repo))
				}
			}
		}
	}
}

// renderNotices writes a ### Notices section if any change has notices.
func renderNotices(b *strings.Builder, changes []ChangeEntry) {
	type pair struct {
		change ChangeEntry
		notice NoticeEntry
	}
	var all []pair
	for _, c := range changes {
		for _, n := range c.Notices {
			all = append(all, pair{c, n})
		}
	}
	if len(all) == 0 {
		return
	}
	b.WriteString("\n### Notices\n\n")
	for _, p := range all {
		fmt.Fprintf(b, "- **%s**: %s\n", strings.ToUpper(p.notice.Level), p.notice.Message)
	}
}

// RenderChangelog produces a single CHANGELOG.md entry in KaC format.
// It includes version header, kind sections (all kinds including chores), notices,
// and reference links. No headline or contributors.
func RenderChangelog(data *ReleaseData, githubRepo string) string {
	var b strings.Builder

	date := ""
	if t, err := time.Parse(time.RFC3339, data.ReleasedAt); err == nil {
		date = t.Format("2006-01-02")
	}
	if date != "" {
		fmt.Fprintf(&b, "## [%s] - %s\n", data.Tag, date)
	} else {
		fmt.Fprintf(&b, "## [%s]\n", data.Tag)
	}

	renderSections(&b, data.Changes, true, nil, false, githubRepo)
	renderNotices(&b, data.Changes)

	if githubRepo != "" && data.PreviousVersion != nil && data.PreviousTag != "" {
		fmt.Fprintf(&b, "\n**Full Changelog: [%s -> %s](https://github.com/%s/compare/%s...%s)**\n",
			data.PreviousTag, data.Tag, githubRepo, data.PreviousTag, data.Tag)
	}

	return b.String()
}

// RenderNotes produces the GitHub Release body: headline (if any), user-facing
// kind sections with PR links, notices, and a contributors footer.
// Chores are excluded. maintainers (lowercased logins) and *[bot] accounts are
// excluded from the contributors footer.
func RenderNotes(data *ReleaseData, maintainers map[string]bool) string {
	var b strings.Builder

	if data.Headline != "" {
		b.WriteString(data.Headline)
		b.WriteString("\n\n")
	}

	renderSections(&b, data.Changes, true, isUserFacing, true, repoSlug(data.Repository))
	renderNotices(&b, data.Changes)

	var thanks []string
	for _, u := range data.Contributors {
		if maintainers[strings.ToLower(u)] || strings.HasSuffix(u, "[bot]") {
			continue
		}
		thanks = append(thanks, "@"+u)
	}
	if len(thanks) > 0 {
		fmt.Fprintf(&b, "\n### Contributors\n\nThanks to %s for contributing to this release!\n", strings.Join(thanks, ", "))
	}

	return b.String()
}

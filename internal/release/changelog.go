package release

import (
	"fmt"
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

// renderSections writes KaC-style ### Kind sections for the given kinds filter.
// Pass nil to include all kinds. When withReleaseNote is set, the authored
// `## Release Note` description lines are rendered under each entry (used for
// the GitHub Release body; the CHANGELOG intentionally shows the title only).
func renderSections(b *strings.Builder, changes []ChangeEntry, withPR bool, filter func(string) bool, withReleaseNote bool) {
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
				pr = fmt.Sprintf(" (#%d)", *e.PR)
			}
			fmt.Fprintf(b, "- %s%s%s\n", e.Title, badge, pr)
			if withReleaseNote && e.ReleaseNote != nil {
				for _, line := range e.ReleaseNote.Description {
					fmt.Fprintf(b, "  %s\n", line)
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

	renderSections(&b, data.Changes, true, nil, false)
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

	renderSections(&b, data.Changes, true, isUserFacing, true)
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

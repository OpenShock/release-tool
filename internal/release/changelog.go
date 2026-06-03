package release

import (
	"fmt"
	"strings"
)

// RenderChangelog derives a CHANGELOG.md entry from a ReleaseData struct.
// maintainers (lowercased logins) and any *[bot] accounts are excluded from
// the Contributors footer; pass nil to apply no maintainer filtering.
func RenderChangelog(data *ReleaseData, githubRepo string, maintainers map[string]bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Version %s Release Notes\n\n", data.Tag)

	if data.Headline != nil {
		b.WriteString(data.Headline.Text)
		b.WriteString("\n\n")
	}

	for _, level := range []string{"major", "minor", "patch"} {
		for _, c := range data.Changes {
			if c.Type != level {
				continue
			}
			var badges []string
			if c.Breaking {
				badges = append(badges, "**BREAKING**")
			}
			if len(c.Categories) > 0 {
				badges = append(badges, "["+strings.Join(c.Categories, ", ")+"]")
			}
			badge := ""
			if len(badges) > 0 {
				badge = " " + strings.Join(badges, " ")
			}
			pr := ""
			if c.PR != nil {
				pr = fmt.Sprintf(" (#%d)", *c.PR)
			}
			fmt.Fprintf(&b, "- %s%s%s\n", c.Title.Text, badge, pr)
			if c.Body != nil {
				for _, line := range strings.Split(c.Body.Text, "\n") {
					if strings.TrimSpace(line) == "" {
						b.WriteByte('\n')
					} else {
						fmt.Fprintf(&b, "  %s\n", line)
					}
				}
			}
		}
	}
	b.WriteByte('\n')

	var allNotices []struct {
		change ChangeEntry
		notice NoticeEntry
	}
	for _, c := range data.Changes {
		for _, n := range c.Notices {
			allNotices = append(allNotices, struct {
				change ChangeEntry
				notice NoticeEntry
			}{c, n})
		}
	}
	if len(allNotices) > 0 {
		b.WriteString("### Notices\n\n")
		for _, an := range allNotices {
			fmt.Fprintf(&b, "- **%s**: %s\n", strings.ToUpper(an.notice.Level), an.notice.Message)
		}
		b.WriteByte('\n')
	}

	var thanks []string
	for _, u := range data.Contributors {
		if maintainers[strings.ToLower(u)] || strings.HasSuffix(u, "[bot]") {
			continue
		}
		thanks = append(thanks, "@"+u)
	}
	if len(thanks) > 0 {
		fmt.Fprintf(&b, "### Contributors\n\nThanks to %s for contributing to this release!\n\n", strings.Join(thanks, ", "))
	}

	if data.PreviousVersion != nil && githubRepo != "" {
		fmt.Fprintf(&b,
			"**Full Changelog: [%s -> %s](https://github.com/%s/compare/%s...%s)**\n\n",
			*data.PreviousVersion, data.Tag, githubRepo, *data.PreviousVersion, data.Tag,
		)
	}

	return b.String()
}

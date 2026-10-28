package release

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/OpenShock/release-tool/internal/changes"
)

// kindOrder is the render order of the kind sections. Safety and security come
// first so the changes a user must not miss are never below the fold.
var kindOrder = []string{"safety", "security", "added", "changed", "deprecated", "removed", "fixed", "chore"}

var kindHeading = map[string]string{
	"safety":     "Safety",
	"security":   "Security",
	"added":      "Added",
	"changed":    "Changed",
	"deprecated": "Deprecated",
	"removed":    "Removed",
	"fixed":      "Fixed",
	"chore":      "Internal",
}

// neverFold are kinds whose sections are always shown in full.
var neverFold = map[string]bool{"safety": true, "security": true}

const (
	// defaultFoldAfter is the section size above which non-breaking entries are
	// folded into a <details> block in the release body.
	defaultFoldAfter = 10
	// defaultPrereleaseNotice opens every prerelease body. "{stable}" is the
	// latest stable tag.
	defaultPrereleaseNotice = "This is a prerelease for testing and may contain bugs. Back up your configuration before installing it."
	stableSentence          = " The latest stable release is {stable}."
)

// NotesOptions configures RenderNotes.
type NotesOptions struct {
	// Maintainers (lowercased logins) are left out of the contributors footer.
	Maintainers map[string]bool
	// PrereleaseNotice replaces the default prerelease warning; "{stable}" is
	// replaced with the latest stable tag.
	PrereleaseNotice string
	// FoldAfter: 0 uses defaultFoldAfter, negative disables folding.
	FoldAfter int
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

// escapeHTML neutralises "<" outside code spans, so a title such as
// "Reject values < 0" renders as written instead of opening an HTML tag.
func escapeHTML(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	parts := strings.Split(text, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = strings.ReplaceAll(parts[i], "<", "&lt;")
	}
	return strings.Join(parts, "`")
}

// inline prepares authored text for a markdown line.
func inline(text, repo string) string { return linkRefs(escapeHTML(text), repo) }

// prLink renders one PR reference, linked when repo is known.
func prLink(pr int, repo string) string {
	if repo == "" {
		return fmt.Sprintf("#%d", pr)
	}
	return fmt.Sprintf("[#%d](https://github.com/%s/pull/%d)", pr, repo, pr)
}

// advisoryLink renders a CVE or GHSA id as a link to its advisory.
func advisoryLink(id string) string {
	if strings.HasPrefix(id, "GHSA-") {
		return fmt.Sprintf("[%s](https://github.com/advisories/%s)", id, id)
	}
	return fmt.Sprintf("[%s](https://www.cve.org/CVERecord?id=%s)", id, id)
}

// extraBlankLines matches two or more consecutive blank lines.
var extraBlankLines = regexp.MustCompile(`\n{3,}`)

// listItem matches markdown list lines ("- x", "* x", "+ x", "1. x").
var listItem = regexp.MustCompile(`^([-*+]|\d+[.)])\s+`)

// repoSlug returns "owner/repo" for data's repository, or "" when unknown.
func repoSlug(r *Repository) string {
	if r == nil || r.Owner == "" || r.Repo == "" {
		return ""
	}
	return r.Owner + "/" + r.Repo
}

// group is one rendered line: changes sharing kind, title, breaking flag and
// release note are merged so a fix landed in several PRs is listed once.
type group struct {
	entry ChangeEntry
	prs   []int
}

func noteKey(n *ReleaseNoteEntry) string {
	if n == nil {
		return ""
	}
	return n.Title + "\x00" + strings.Join(n.Description, "\x00")
}

func groupEntries(entries []ChangeEntry) []*group {
	var out []*group
	index := map[string]*group{}
	for _, e := range entries {
		key := fmt.Sprintf("%t\x00%s\x00%s", e.Breaking, e.Title, noteKey(e.ReleaseNote))
		g, ok := index[key]
		if !ok {
			g = &group{entry: e}
			index[key] = g
			out = append(out, g)
		} else {
			g.entry.Advisories = appendUnique(g.entry.Advisories, e.Advisories...)
		}
		if e.PR != nil {
			g.prs = append(g.prs, *e.PR)
		}
	}
	return out
}

// writeEntry renders one list item. withReleaseNote adds the authored
// `## Release Note` description under it: prose lines inline, list items in a
// collapsed <details> block so implementation detail stays out of the way
// without being lost.
func writeEntry(b *strings.Builder, g *group, repo string, withPR, withReleaseNote bool) {
	e := g.entry
	prefix := ""
	if e.Breaking {
		prefix = "**Breaking:** "
	}
	var refs []string
	if withPR {
		for _, pr := range g.prs {
			refs = append(refs, prLink(pr, repo))
		}
	}
	for _, a := range e.Advisories {
		refs = append(refs, advisoryLink(a))
	}
	suffix := ""
	if len(refs) > 0 {
		suffix = " (" + strings.Join(refs, ", ") + ")"
	}
	fmt.Fprintf(b, "- %s%s%s\n", prefix, inline(e.Title, repo), suffix)

	if !withReleaseNote || e.ReleaseNote == nil {
		return
	}
	var details []string
	for _, line := range e.ReleaseNote.Description {
		if listItem.MatchString(line) {
			details = append(details, line)
			continue
		}
		fmt.Fprintf(b, "  %s\n", inline(line, repo))
	}
	if len(details) > 0 {
		b.WriteString("  <details><summary>Details</summary>\n\n")
		for _, line := range details {
			fmt.Fprintf(b, "  %s\n", inline(line, repo))
		}
		b.WriteString("\n  </details>\n")
	}
}

// renderSections writes ### Kind sections for the kinds filter (nil = all).
// When withReleaseNote is set (the GitHub Release body), each entry carries its
// authored description and long sections are folded after foldAfter entries
// (<= 0 disables). repo ("owner/repo", may be empty) enables links.
func renderSections(b *strings.Builder, changes []ChangeEntry, withPR bool, filter func(string) bool, withReleaseNote bool, repo string, foldAfter int) {
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

		var shown, folded []*group
		groups := groupEntries(entries)
		fold := withReleaseNote && foldAfter > 0 && !neverFold[kind] && len(groups) > foldAfter
		for _, g := range groups {
			if fold && !g.entry.Breaking {
				folded = append(folded, g)
			} else {
				shown = append(shown, g)
			}
		}
		for _, g := range shown {
			writeEntry(b, g, repo, withPR, withReleaseNote)
		}
		if len(folded) > 0 {
			label := "changes"
			if len(shown) > 0 {
				label = "more changes"
			}
			fmt.Fprintf(b, "\n<details><summary>%d %s</summary>\n\n", len(folded), label)
			for _, g := range folded {
				writeEntry(b, g, repo, withPR, withReleaseNote)
			}
			b.WriteString("\n</details>\n")
		}
	}
}

// alertItems collects what a reader must see before installing, keyed by
// notice level (most severe first in changes.NoticeLevels). withBreaking adds
// a line per breaking change; the CHANGELOG leaves that out because its
// entries already carry the **Breaking:** prefix.
func alertItems(data *ReleaseData, repo string, withBreaking bool) map[string][]string {
	items := map[string][]string{}
	add := func(level, text string) { items[level] = append(items[level], text) }

	if withBreaking {
		for _, c := range data.Changes {
			if c.Kind == "safety" {
				add("caution", "This release changes safety-related behaviour. Read the **Safety** section before installing it.")
				break
			}
		}
	}
	for _, v := range data.Yanks {
		add("warning", fmt.Sprintf("**Version %s has been withdrawn.** If you installed it, update to this release.", v))
	}
	if withBreaking {
		for _, c := range data.Changes {
			if c.Breaking && isUserFacing(c.Kind) {
				add("warning", "**Breaking:** "+inline(c.Title, repo))
			}
		}
	}
	if data.Mandatory {
		add("important", "**Required update:** install this version before updating to any later version.")
	}
	if u := data.Upgrade; u != nil {
		if u.MinVersion != "" {
			add("important", fmt.Sprintf("**Update to %s or later first.** This version cannot be installed directly from older versions.", u.MinVersion))
		}
		if u.USBOnly {
			add("important", "**Install over USB.** This version cannot be installed over the air (OTA).")
		}
		if u.ConfigReset {
			add("important", "**Back up your configuration.** This update resets it.")
		}
		if u.NoDowngrade {
			add("important", "**No downgrade.** After this update you cannot go back to an earlier version.")
		}
	}
	for _, c := range data.Changes {
		for _, n := range c.Notices {
			add(n.Level, inline(n.Message, repo))
		}
	}
	return items
}

// writeAlert renders a GitHub alert block ("> [!WARNING]") holding items.
func writeAlert(b *strings.Builder, level string, items []string) {
	fmt.Fprintf(b, "> [!%s]\n", strings.ToUpper(level))
	if len(items) == 1 {
		fmt.Fprintf(b, "> %s\n\n", items[0])
		return
	}
	for _, it := range items {
		fmt.Fprintf(b, "> - %s\n", it)
	}
	b.WriteString("\n")
}

// prereleaseNotice expands the prerelease warning template.
func prereleaseNotice(tmpl, stable string) string {
	if tmpl == "" {
		tmpl = defaultPrereleaseNotice
		if stable != "" {
			tmpl += stableSentence
		}
	}
	if stable == "" {
		stable = "not available yet"
	}
	return strings.ReplaceAll(tmpl, "{stable}", stable)
}

// RenderChangelog produces a single CHANGELOG.md entry in KaC format: version
// header, notices, kind sections (chores last, as Internal) and the compare
// link. No headline or contributors.
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

	items := alertItems(data, githubRepo, false)
	if len(items) > 0 {
		b.WriteString("\n### Notices\n\n")
		for _, level := range changes.NoticeLevels {
			for _, it := range items[level] {
				fmt.Fprintf(&b, "- **%s**: %s\n", strings.ToUpper(level), it)
			}
		}
	}

	renderSections(&b, data.Changes, true, nil, false, githubRepo, 0)

	if githubRepo != "" && data.PreviousVersion != nil && data.PreviousTag != "" {
		fmt.Fprintf(&b, "\n**Full Changelog: [%s -> %s](https://github.com/%s/compare/%s...%s)**\n",
			data.PreviousTag, data.Tag, githubRepo, data.PreviousTag, data.Tag)
	}

	return b.String()
}

// RenderNotes produces the GitHub Release body, ordered by what a reader needs
// before installing: prerelease warning, headline, alerts (safety, breaking
// changes, upgrade requirements and notices), what is new since the previous
// prerelease, the user-facing kind sections, known issues, contributors and
// the compare link. Chores are excluded.
func RenderNotes(data *ReleaseData, opts NotesOptions) string {
	var b strings.Builder
	repo := repoSlug(data.Repository)

	if data.Prerelease {
		writeAlert(&b, "warning", []string{prereleaseNotice(opts.PrereleaseNotice, data.PreviousTag)})
	}

	if data.Headline != "" {
		b.WriteString(data.Headline)
		b.WriteString("\n\n")
	}

	items := alertItems(data, repo, true)
	for _, level := range changes.NoticeLevels {
		if len(items[level]) > 0 {
			writeAlert(&b, level, items[level])
		}
	}

	if d := data.SincePrerelease; d != nil {
		fmt.Fprintf(&b, "### New since %s\n\n", d.Tag)
		isNew := make(map[string]bool, len(d.Changes))
		for _, id := range d.Changes {
			isNew[id] = true
		}
		n := 0
		for _, c := range data.Changes {
			if isNew[c.ID] && isUserFacing(c.Kind) {
				fmt.Fprintf(&b, "- %s\n", inline(c.Title, repo))
				n++
			}
		}
		if n == 0 {
			fmt.Fprintf(&b, "No user-facing changes since %s.\n", d.Tag)
		}
	}

	foldAfter := opts.FoldAfter
	if foldAfter == 0 {
		foldAfter = defaultFoldAfter
	}
	renderSections(&b, data.Changes, true, isUserFacing, true, repo, foldAfter)

	if len(data.KnownIssues) > 0 {
		b.WriteString("\n### Known issues\n")
		for _, issue := range data.KnownIssues {
			fmt.Fprintf(&b, "- %s\n", inline(issue, repo))
		}
	}

	credit := func(logins []string) []string {
		var out []string
		for _, u := range logins {
			if opts.Maintainers[strings.ToLower(u)] || strings.HasSuffix(u, "[bot]") {
				continue
			}
			out = append(out, "@"+u)
		}
		return out
	}
	thanks := credit(data.Contributors)
	newcomers := credit(data.FirstTimeContributors)
	if len(thanks) > 0 {
		fmt.Fprintf(&b, "\n### Contributors\n\nThanks to %s for contributing to this release!\n", strings.Join(thanks, ", "))
		if len(newcomers) > 0 {
			fmt.Fprintf(&b, "\nWelcome to %s, who made their first contribution.\n", strings.Join(newcomers, ", "))
		}
	}

	if repo != "" && data.PreviousTag != "" {
		fmt.Fprintf(&b, "\n**Full Changelog**: https://github.com/%s/compare/%s...%s\n", repo, data.PreviousTag, data.Tag)
	}

	// Blocks each end with their own spacing; collapse the runs where two meet.
	return strings.TrimLeft(extraBlankLines.ReplaceAllString(b.String(), "\n\n"), "\n")
}

// MarkYanked appends " [YANKED]" (Keep a Changelog) to the CHANGELOG headings
// of the given versions. Headings already marked are left as they are.
func MarkYanked(changelog, tagPrefix string, versions []string) string {
	for _, v := range versions {
		re := regexp.MustCompile(`(?m)^(## \[` + regexp.QuoteMeta(tagPrefix+v) + `\][^\r\n]*?)( \[YANKED\])?(\r?)$`)
		changelog = re.ReplaceAllString(changelog, "${1} [YANKED]${3}")
	}
	return changelog
}

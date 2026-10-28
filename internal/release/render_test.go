package release

import (
	"reflect"
	"strings"
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func TestRenderNotes_LayoutOrder(t *testing.T) {
	pr := 7
	d := &ReleaseData{
		Tag:             "1.6.0-rc.3",
		Prerelease:      true,
		PreviousVersion: ptr("1.5.2"),
		PreviousTag:     "1.5.2",
		Repository:      &Repository{Platform: "github", Owner: "o", Repo: "r"},
		Headline:        "Big release.",
		Mandatory:       true,
		Upgrade:         &changes.Upgrade{MinVersion: "1.4.0"},
		SincePrerelease: &PrereleaseDelta{Tag: "1.6.0-rc.2", Changes: []string{"new-fix"}},
		Changes: []ChangeEntry{
			{ID: "estop", Kind: "safety", Title: "Refuse X", Notices: []NoticeEntry{}},
			{ID: "drop", Kind: "removed", Breaking: true, Title: "Remove Y", Notices: []NoticeEntry{{Level: "caution", Message: "Hold on"}}},
			{ID: "new-fix", Kind: "fixed", Title: "Fix Z", PR: &pr, Notices: []NoticeEntry{{Level: "note", Message: "FYI"}}},
		},
		KnownIssues:           []string{"Pairing fails on USB power."},
		Contributors:          []string{"alice", "bob"},
		FirstTimeContributors: []string{"bob"},
	}
	notes := RenderNotes(d, NotesOptions{})

	order := []string{
		"> [!WARNING]\n> This is a prerelease",
		"The latest stable release is 1.5.2.",
		"Big release.",
		"> [!CAUTION]",
		"> - This release changes safety-related behaviour",
		"> - Hold on",
		"> [!WARNING]\n> **Breaking:** Remove Y",
		"> [!IMPORTANT]",
		"**Required update:**",
		"**Update to 1.4.0 or later first.**",
		"> [!NOTE]\n> FYI",
		"### New since 1.6.0-rc.2\n\n- Fix Z\n",
		"### Safety",
		"### Removed\n- **Breaking:** Remove Y",
		"### Fixed\n- Fix Z ([#7](https://github.com/o/r/pull/7))",
		"### Known issues\n- Pairing fails on USB power.",
		"Thanks to @alice, @bob",
		"Welcome to @bob, who made their first contribution.",
		"**Full Changelog**: https://github.com/o/r/compare/1.5.2...1.6.0-rc.3",
	}
	last := -1
	for _, s := range order {
		i := strings.Index(notes, s)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", s, notes)
		}
		if i < last {
			t.Errorf("%q is out of order in:\n%s", s, notes)
		}
		last = i
	}
}

func TestRenderNotes_PrereleaseNotice(t *testing.T) {
	d := &ReleaseData{Tag: "1.0.0-rc.1", Prerelease: true, Changes: []ChangeEntry{}}
	if n := RenderNotes(d, NotesOptions{}); strings.Contains(n, "latest stable") {
		t.Errorf("no stable sentence without a previous stable:\n%s", n)
	}
	d.PreviousTag = "0.9.0"
	n := RenderNotes(d, NotesOptions{PrereleaseNotice: "Only install if you can reflash over USB. Stable: {stable}."})
	if !strings.HasPrefix(n, "> [!WARNING]\n> Only install if you can reflash over USB. Stable: 0.9.0.\n") {
		t.Errorf("custom notice not used:\n%s", n)
	}
	d.Prerelease = false
	if n := RenderNotes(d, NotesOptions{}); strings.Contains(n, "prerelease") {
		t.Errorf("stable release must not carry the prerelease notice:\n%s", n)
	}
}

func TestRenderNotes_NothingNewSincePrerelease(t *testing.T) {
	d := baseData()
	d.SincePrerelease = &PrereleaseDelta{Tag: "v1.3.0-rc.1", Changes: []string{}}
	if n := RenderNotes(d, NotesOptions{}); !strings.Contains(n, "No user-facing changes since v1.3.0-rc.1.") {
		t.Errorf("expected an explicit nothing-new line:\n%s", n)
	}
}

func TestRenderNotes_FoldsLongSections(t *testing.T) {
	d := &ReleaseData{Tag: "v2.0.0"}
	for i := 0; i < 4; i++ {
		d.Changes = append(d.Changes, ChangeEntry{Kind: "fixed", Title: "Fix " + string(rune('a'+i)), Notices: []NoticeEntry{}})
		d.Changes = append(d.Changes, ChangeEntry{Kind: "safety", Title: "Safe " + string(rune('a'+i)), Notices: []NoticeEntry{}})
	}
	d.Changes = append(d.Changes, ChangeEntry{Kind: "fixed", Breaking: true, Title: "Break it", ReleaseNote: &ReleaseNoteEntry{Title: "x"}, Notices: []NoticeEntry{}})

	n := RenderNotes(d, NotesOptions{FoldAfter: 3})
	if !strings.Contains(n, "### Fixed\n- **Breaking:** Break it\n\n<details><summary>4 more changes</summary>\n\n- Fix a\n") {
		t.Errorf("non-breaking fixes should fold below the breaking one:\n%s", n)
	}
	if strings.Contains(n, "### Safety\n\n<details>") || !strings.Contains(n, "### Safety\n- Safe a\n") {
		t.Errorf("safety must never fold:\n%s", n)
	}
	if strings.Contains(RenderNotes(d, NotesOptions{FoldAfter: -1}), "<details>") {
		t.Error("a negative FoldAfter disables folding")
	}
}

func TestRenderSections_MergesDuplicateTitles(t *testing.T) {
	a, b := 1, 2
	d := &ReleaseData{Tag: "v1.0.0", Changes: []ChangeEntry{
		{Kind: "fixed", Title: "Fix boot", PR: &a, Notices: []NoticeEntry{}},
		{Kind: "fixed", Title: "Fix boot", PR: &b, Notices: []NoticeEntry{}},
		{Kind: "added", Title: "Fix boot", Notices: []NoticeEntry{}},
	}}
	n := RenderChangelog(d, "")
	if !strings.Contains(n, "- Fix boot (#1, #2)\n") || strings.Count(n, "Fix boot") != 2 {
		t.Errorf("same kind and title should merge into one line with both PRs:\n%s", n)
	}
}

func TestRenderEntry_AdvisoriesAndEscaping(t *testing.T) {
	d := &ReleaseData{Tag: "v1.0.0", Changes: []ChangeEntry{
		{Kind: "security", Title: "Reject sizes < 4 KB, not `<4096`", Advisories: []string{"CVE-2026-1234", "GHSA-jfh8-c2jp-5v3q"}, Notices: []NoticeEntry{}},
	}}
	n := RenderChangelog(d, "")
	want := "- Reject sizes &lt; 4 KB, not `<4096` ([CVE-2026-1234](https://www.cve.org/CVERecord?id=CVE-2026-1234), [GHSA-jfh8-c2jp-5v3q](https://github.com/advisories/GHSA-jfh8-c2jp-5v3q))"
	if !strings.Contains(n, want) {
		t.Errorf("want %q in:\n%s", want, n)
	}
}

func TestRenderChangelog_NoticesAtTop(t *testing.T) {
	d := baseData()
	d.Yanks = []string{"1.2.1"}
	d.Upgrade = &changes.Upgrade{USBOnly: true}
	d.Changes[1].Notices = []NoticeEntry{{Level: "warning", Message: "Heads up"}}
	n := RenderChangelog(d, "")
	order := []string{"### Notices", "- **WARNING**: **Version 1.2.1 has been withdrawn.**", "- **WARNING**: Heads up", "- **IMPORTANT**: **Install over USB.**", "### Added"}
	last := -1
	for _, s := range order {
		i := strings.Index(n, s)
		if i < last || i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", s, n)
		}
		last = i
	}
}

func TestMarkYanked(t *testing.T) {
	in := "## [v1.2.1] - 2026-01-01\r\n### Fixed\n## [v1.2.0] - 2025-12-01\n## [v1.2.10] - 2026-02-01\n## [v1.1.0] - 2025-01-01 [YANKED]\n"
	got := MarkYanked(in, "v", []string{"1.2.1", "1.1.0"})
	want := "## [v1.2.1] - 2026-01-01 [YANKED]\r\n### Fixed\n## [v1.2.0] - 2025-12-01\n## [v1.2.10] - 2026-02-01\n## [v1.1.0] - 2025-01-01 [YANKED]\n"
	if got != want {
		t.Errorf("MarkYanked:\ngot  %q\nwant %q", got, want)
	}
}

func TestBuildData_AggregatesExtras(t *testing.T) {
	ch := []*changes.Change{
		{Kind: "fixed", Title: "A", Filename: "a.md", Upgrade: changes.Upgrade{MinVersion: "1.4.0", USBOnly: true}, Authors: []string{"carol"}, Yanks: []string{"1.5.1"}},
		{Kind: "chore", Title: "B", Filename: "b.md", Upgrade: changes.Upgrade{MinVersion: "1.10.0", ConfigReset: true}, Authors: []string{"Carol", "dave"}, Yanks: []string{"1.5.1"}},
	}
	data := BuildData(BuildParams{
		Tag: "1.6.0-rc.3", Version: "1.6.0", Changes: ch, Prerelease: true,
		KnownIssues: []string{"Known"}, PreviousPrerelease: "1.6.0-rc.2", NewSincePrerelease: []string{"b.md", "gone.md"},
	})
	if want := (changes.Upgrade{MinVersion: "1.10.0", USBOnly: true, ConfigReset: true}); data.Upgrade == nil || *data.Upgrade != want {
		t.Errorf("Upgrade = %+v, want %+v (highest version, OR-ed flags)", data.Upgrade, want)
	}
	if !reflect.DeepEqual(data.Yanks, []string{"1.5.1"}) {
		t.Errorf("Yanks = %v", data.Yanks)
	}
	if !reflect.DeepEqual(data.Contributors, []string{"carol", "dave"}) {
		t.Errorf("Contributors = %v (credited authors, deduplicated case-insensitively)", data.Contributors)
	}
	if !reflect.DeepEqual(data.KnownIssues, []string{"Known"}) {
		t.Errorf("KnownIssues = %v", data.KnownIssues)
	}
	if d := data.SincePrerelease; d == nil || d.Tag != "1.6.0-rc.2" || !reflect.DeepEqual(d.Changes, []string{"b"}) {
		t.Errorf("SincePrerelease = %+v", d)
	}
	if data.Changes[0].Audience != "user" || data.Changes[1].Audience != "internal" {
		t.Errorf("audience: %q, %q", data.Changes[0].Audience, data.Changes[1].Audience)
	}
	if data.FirstTimeContributors == nil || data.KnownIssues == nil {
		t.Error("list fields must serialise as [] rather than null")
	}
}

func TestBuildData_NoUpgradeWhenUnset(t *testing.T) {
	data := BuildData(BuildParams{Tag: "1.0.0", Changes: []*changes.Change{{Kind: "fixed", Title: "A", Filename: "a.md"}}})
	if data.Upgrade != nil || data.SincePrerelease != nil {
		t.Errorf("unset extras should be omitted: upgrade=%+v since=%+v", data.Upgrade, data.SincePrerelease)
	}
}

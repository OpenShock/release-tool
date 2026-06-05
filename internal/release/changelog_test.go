package release

import (
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func baseData() *ReleaseData {
	return &ReleaseData{
		Tag:             "v1.3.0",
		PreviousVersion: ptr("1.2.0"),
		PreviousTag:     "v1.2.0",
		ReleasedAt:      "2026-06-05T00:00:00Z",
		Changes: []ChangeEntry{
			{
				ID:      "add-feature",
				Kind:    "added",
				Title:   "Add new feature",
				Notices: []NoticeEntry{},
			},
			{
				ID:      "fix-bug",
				Kind:    "fixed",
				Title:   "Fix crash",
				Notices: []NoticeEntry{},
			},
		},
	}
}

func TestRenderChangelog_Header(t *testing.T) {
	entry := RenderChangelog(baseData(), "")
	if !strings.Contains(entry, "## [v1.3.0] - 2026-06-05") {
		t.Errorf("missing header in:\n%s", entry)
	}
}

func TestRenderChangelog_KindSections(t *testing.T) {
	entry := RenderChangelog(baseData(), "")
	if !strings.Contains(entry, "### Added") {
		t.Errorf("missing Added section in:\n%s", entry)
	}
	if !strings.Contains(entry, "### Fixed") {
		t.Errorf("missing Fixed section in:\n%s", entry)
	}
	if strings.Contains(entry, "### Changed") {
		t.Errorf("empty Changed section should be omitted in:\n%s", entry)
	}
}

func TestRenderChangelog_KindOrder(t *testing.T) {
	d := &ReleaseData{
		Tag:        "v2.0.0",
		ReleasedAt: "2026-01-01T00:00:00Z",
		Changes: []ChangeEntry{
			{Kind: "fixed", Title: "Patch fix", Notices: []NoticeEntry{}},
			{Kind: "removed", Title: "Removed item", Notices: []NoticeEntry{}},
			{Kind: "added", Title: "New thing", Notices: []NoticeEntry{}},
		},
	}
	entry := RenderChangelog(d, "")
	addedIdx := strings.Index(entry, "New thing")
	removedIdx := strings.Index(entry, "Removed item")
	fixedIdx := strings.Index(entry, "Patch fix")
	if !(addedIdx < removedIdx && removedIdx < fixedIdx) {
		t.Errorf("expected added < removed < fixed order in:\n%s", entry)
	}
}

func TestRenderChangelog_BreakingBadge(t *testing.T) {
	d := &ReleaseData{
		Tag:        "v2.0.0",
		ReleasedAt: "2026-01-01T00:00:00Z",
		Changes: []ChangeEntry{
			{Kind: "removed", Breaking: true, Title: "Drop API", Notices: []NoticeEntry{}},
		},
	}
	entry := RenderChangelog(d, "")
	if !strings.Contains(entry, "**BREAKING**") {
		t.Errorf("expected BREAKING badge in:\n%s", entry)
	}
}

func TestRenderChangelog_PRLink(t *testing.T) {
	pr := 42
	d := &ReleaseData{
		Tag:        "v1.1.0",
		ReleasedAt: "2026-01-01T00:00:00Z",
		Changes: []ChangeEntry{
			{Kind: "added", PR: &pr, Title: "New feature", Notices: []NoticeEntry{}},
		},
	}
	entry := RenderChangelog(d, "")
	if !strings.Contains(entry, "(#42)") {
		t.Errorf("expected PR link in:\n%s", entry)
	}
}

func TestRenderChangelog_Notices(t *testing.T) {
	d := &ReleaseData{
		Tag:        "v1.1.0",
		ReleasedAt: "2026-01-01T00:00:00Z",
		Changes: []ChangeEntry{
			{
				Kind:  "changed",
				Title: "Feature",
				Notices: []NoticeEntry{
					{Level: "warning", Message: "requires restart"},
					{Level: "info", Message: "optional migration"},
				},
			},
		},
	}
	entry := RenderChangelog(d, "")
	if !strings.Contains(entry, "### Notices") {
		t.Errorf("expected Notices section in:\n%s", entry)
	}
	if !strings.Contains(entry, "**WARNING**") {
		t.Errorf("expected WARNING notice in:\n%s", entry)
	}
	if !strings.Contains(entry, "requires restart") {
		t.Errorf("expected notice message in:\n%s", entry)
	}
}

func TestRenderChangelog_FullChangelogLink(t *testing.T) {
	d := baseData()
	entry := RenderChangelog(d, "OpenShock/Firmware")
	if !strings.Contains(entry, "**Full Changelog: [v1.2.0 -> v1.3.0](https://github.com/OpenShock/Firmware/compare/v1.2.0...v1.3.0)**") {
		t.Errorf("expected full changelog link in:\n%s", entry)
	}
}

func TestRenderChangelog_NoLinkForFirstRelease(t *testing.T) {
	d := baseData()
	d.PreviousVersion = nil
	d.PreviousTag = ""
	entry := RenderChangelog(d, "OpenShock/Firmware")
	if strings.Contains(entry, "Full Changelog") {
		t.Errorf("first release should have no changelog link:\n%s", entry)
	}
}

func TestRenderChangelog_NoLinksWithoutRepo(t *testing.T) {
	entry := RenderChangelog(baseData(), "")
	if strings.Contains(entry, "github.com") {
		t.Errorf("should not include links when githubRepo is empty:\n%s", entry)
	}
}

// --- RenderNotes tests ---

func TestRenderNotes_Headline(t *testing.T) {
	d := baseData()
	d.Headline = "Exciting release"
	entry := RenderNotes(d, nil)
	if !strings.Contains(entry, "Exciting release") {
		t.Errorf("missing headline in:\n%s", entry)
	}
}

func TestRenderNotes_NoHeadline(t *testing.T) {
	d := baseData()
	d.Headline = ""
	entry := RenderNotes(d, nil)
	if !strings.Contains(entry, "### Added") {
		t.Errorf("sections should still appear without headline:\n%s", entry)
	}
}

func TestRenderNotes_Contributors(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"alice", "bob"}
	entry := RenderNotes(d, nil)
	if !strings.Contains(entry, "### Contributors") {
		t.Errorf("expected Contributors heading in:\n%s", entry)
	}
	if !strings.Contains(entry, "Thanks to @alice, @bob for contributing") {
		t.Errorf("expected thanks line in:\n%s", entry)
	}
}

func TestRenderNotes_ContributorsExcludesMaintainersAndBots(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"alice", "Maintainer", "dependabot[bot]"}
	maintainers := map[string]bool{"maintainer": true}
	entry := RenderNotes(d, maintainers)
	if !strings.Contains(entry, "@alice") {
		t.Errorf("expected @alice in:\n%s", entry)
	}
	if strings.Contains(entry, "Maintainer") {
		t.Errorf("maintainer should be excluded in:\n%s", entry)
	}
	if strings.Contains(entry, "dependabot") {
		t.Errorf("bot should be excluded in:\n%s", entry)
	}
}

func TestRenderNotes_NoContributorsSectionWhenAllFiltered(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"onlymaintainer", "ci[bot]"}
	entry := RenderNotes(d, map[string]bool{"onlymaintainer": true})
	if strings.Contains(entry, "Thanks to") {
		t.Errorf("should omit thanks when everyone is filtered:\n%s", entry)
	}
}


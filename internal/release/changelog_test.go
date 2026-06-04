package release

import (
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func baseData() *ReleaseData {
	return &ReleaseData{
		Tag:             "1.3.0",
		PreviousVersion: ptr("1.2.0"),
		Changes: []ChangeEntry{
			{
				ID:         "add-feature",
				Type:       "minor",
				Breaking:   false,
				Categories: []string{},
				Title:      MDText{Format: "markdown", Text: "Add new feature"},
				Notices:    []NoticeEntry{},
			},
			{
				ID:         "fix-bug",
				Type:       "patch",
				Breaking:   false,
				Categories: []string{},
				Title:      MDText{Format: "markdown", Text: "Fix crash"},
				Notices:    []NoticeEntry{},
			},
		},
	}
}

func TestRenderChangelog_Header(t *testing.T) {
	entry := RenderChangelog(baseData(), "", nil)
	if !strings.Contains(entry, "# Version 1.3.0 Release Notes") {
		t.Errorf("missing header in:\n%s", entry)
	}
}

func TestRenderChangelog_Headline(t *testing.T) {
	d := baseData()
	d.Headline = &MDText{Format: "markdown", Text: "Exciting release with new things"}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "Exciting release with new things") {
		t.Errorf("missing headline in:\n%s", entry)
	}
}

func TestRenderChangelog_NoHeadline(t *testing.T) {
	d := baseData()
	d.Headline = nil
	entry := RenderChangelog(d, "", nil)
	// header should still be present; no stray blank sections
	if !strings.Contains(entry, "# Version") {
		t.Errorf("missing header")
	}
}

func TestRenderChangelog_ChangeOrder(t *testing.T) {
	d := &ReleaseData{
		Tag: "2.0.0",
		Changes: []ChangeEntry{
			{Type: "patch", Title: MDText{Text: "Patch fix"}, Notices: []NoticeEntry{}, Categories: []string{}},
			{Type: "major", Title: MDText{Text: "Major change"}, Notices: []NoticeEntry{}, Categories: []string{}},
			{Type: "minor", Title: MDText{Text: "Minor add"}, Notices: []NoticeEntry{}, Categories: []string{}},
		},
	}
	entry := RenderChangelog(d, "", nil)
	majorIdx := strings.Index(entry, "Major change")
	minorIdx := strings.Index(entry, "Minor add")
	patchIdx := strings.Index(entry, "Patch fix")
	if !(majorIdx < minorIdx && minorIdx < patchIdx) {
		t.Errorf("expected major < minor < patch order in:\n%s", entry)
	}
}

func TestRenderChangelog_BreakingBadge(t *testing.T) {
	d := &ReleaseData{
		Tag: "2.0.0",
		Changes: []ChangeEntry{
			{Type: "major", Breaking: true, Title: MDText{Text: "Drop API"}, Notices: []NoticeEntry{}, Categories: []string{}},
		},
	}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "**BREAKING**") {
		t.Errorf("expected BREAKING badge in:\n%s", entry)
	}
}

func TestRenderChangelog_CategoryBadge(t *testing.T) {
	d := &ReleaseData{
		Tag: "1.1.0",
		Changes: []ChangeEntry{
			{Type: "minor", Categories: []string{"api", "firmware"}, Title: MDText{Text: "New endpoint"}, Notices: []NoticeEntry{}},
		},
	}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "[api, firmware]") {
		t.Errorf("expected category badge in:\n%s", entry)
	}
}

func TestRenderChangelog_PRLink(t *testing.T) {
	pr := 42
	d := &ReleaseData{
		Tag: "1.1.0",
		Changes: []ChangeEntry{
			{Type: "minor", PR: &pr, Title: MDText{Text: "New feature"}, Notices: []NoticeEntry{}, Categories: []string{}},
		},
	}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "(#42)") {
		t.Errorf("expected PR link in:\n%s", entry)
	}
}

func TestRenderChangelog_OnlyTitleInChangelog(t *testing.T) {
	d := &ReleaseData{
		Tag: "1.1.0",
		Changes: []ChangeEntry{
			{Type: "minor", Title: MDText{Text: "Feature"}, Notices: []NoticeEntry{}, Categories: []string{}},
		},
	}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "- Feature") {
		t.Errorf("title should appear as bullet in changelog:\n%s", entry)
	}
	if !strings.Contains(entry, "Feature") {
		t.Errorf("title should still appear in changelog:\n%s", entry)
	}
}

func TestRenderChangelog_Notices(t *testing.T) {
	d := &ReleaseData{
		Tag: "1.1.0",
		Changes: []ChangeEntry{
			{
				Type:       "minor",
				Title:      MDText{Text: "Feature"},
				Categories: []string{},
				Notices: []NoticeEntry{
					{Level: "warning", Message: "requires restart"},
					{Level: "info", Message: "optional migration"},
				},
			},
		},
	}
	entry := RenderChangelog(d, "", nil)
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

func TestRenderChangelog_FullDiffLink(t *testing.T) {
	d := baseData()
	entry := RenderChangelog(d, "OpenShock/backend", nil)
	if !strings.Contains(entry, "https://github.com/OpenShock/backend/compare/1.2.0...1.3.0") {
		t.Errorf("expected full-diff link in:\n%s", entry)
	}
}

func TestRenderChangelog_NoFullDiffLinkWhenNoPrevious(t *testing.T) {
	d := baseData()
	d.PreviousVersion = nil
	entry := RenderChangelog(d, "OpenShock/backend", nil)
	if strings.Contains(entry, "compare/") {
		t.Errorf("should not include diff link when PreviousVersion is nil")
	}
}

func TestRenderChangelog_NoFullDiffLinkWhenNoRepo(t *testing.T) {
	d := baseData()
	entry := RenderChangelog(d, "", nil) // no github-repo
	if strings.Contains(entry, "compare/") {
		t.Errorf("should not include diff link when githubRepo is empty")
	}
}

func TestRenderChangelog_Contributors(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"alice", "bob"}
	entry := RenderChangelog(d, "", nil)
	if !strings.Contains(entry, "### Contributors") {
		t.Errorf("expected Contributors section in:\n%s", entry)
	}
	if !strings.Contains(entry, "Thanks to @alice, @bob for contributing") {
		t.Errorf("expected thanks line in:\n%s", entry)
	}
}

func TestRenderChangelog_ContributorsExcludesMaintainersAndBots(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"alice", "Maintainer", "dependabot[bot]"}
	maintainers := map[string]bool{"maintainer": true} // lowercased
	entry := RenderChangelog(d, "", maintainers)
	if !strings.Contains(entry, "@alice") {
		t.Errorf("expected @alice to be thanked in:\n%s", entry)
	}
	if strings.Contains(entry, "Maintainer") {
		t.Errorf("maintainer should be excluded (case-insensitive) in:\n%s", entry)
	}
	if strings.Contains(entry, "dependabot") {
		t.Errorf("bot should be excluded in:\n%s", entry)
	}
}

func TestRenderChangelog_NoContributorsSectionWhenAllFiltered(t *testing.T) {
	d := baseData()
	d.Contributors = []string{"onlymaintainer", "ci[bot]"}
	entry := RenderChangelog(d, "", map[string]bool{"onlymaintainer": true})
	if strings.Contains(entry, "### Contributors") {
		t.Errorf("should omit Contributors section when everyone is filtered:\n%s", entry)
	}
}

func TestRenderChangelog_NoContributorsSectionWhenEmpty(t *testing.T) {
	d := baseData()
	d.Contributors = nil
	entry := RenderChangelog(d, "", nil)
	if strings.Contains(entry, "### Contributors") {
		t.Errorf("should omit Contributors section when none present:\n%s", entry)
	}
}

package release

import (
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func makeChange(kind, title string, breaking bool) *changes.Change {
	return &changes.Change{
		Kind:        kind,
		Bump:        changes.DeriveBump(kind, breaking),
		Title:       title,
		ReleaseNote: "release note text",
		Breaking:    breaking,
		Filename:    title + ".md",
		Notices:     []changes.Notice{{Level: "warning", Message: "heads up"}},
	}
}

func TestBuildData_Fields(t *testing.T) {
	ch := []*changes.Change{
		makeChange("added", "Add feature", false),
	}
	p := BuildParams{
		Tag:        "1.3.0",
		Previous:   "1.2.0",
		Changes:    ch,
		Headline:   "Big release",
		Prerelease: false,
		Commit:     "abc123",
		Version:    "1.3.0",
		EnrichPR:   false,
	}
	data := BuildData(p)

	if data.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d, want 1", data.SchemaVersion)
	}
	if data.Tag != "1.3.0" {
		t.Errorf("Tag: got %q", data.Tag)
	}
	if data.Version != "1.3.0" {
		t.Errorf("Version: got %q", data.Version)
	}
	if data.Prerelease {
		t.Error("Prerelease should be false")
	}
	if data.PreviousVersion == nil || *data.PreviousVersion != "1.2.0" {
		t.Errorf("PreviousVersion: got %v", data.PreviousVersion)
	}
	if data.Commit != "abc123" {
		t.Errorf("Commit: got %q", data.Commit)
	}
	if data.Headline != "Big release" {
		t.Errorf("Headline: got %q", data.Headline)
	}
	if data.ReleasedAt == "" {
		t.Error("ReleasedAt should not be empty")
	}
}

func TestBuildData_Changes(t *testing.T) {
	ch := []*changes.Change{
		makeChange("removed", "Breaking change", true),
	}
	data := BuildData(BuildParams{
		Tag:      "2.0.0",
		Changes:  ch,
		EnrichPR: false,
	})

	if len(data.Changes) != 1 {
		t.Fatalf("expected 1 change entry, got %d", len(data.Changes))
	}
	e := data.Changes[0]
	if e.Kind != "removed" {
		t.Errorf("Kind: got %q", e.Kind)
	}
	if !e.Breaking {
		t.Error("Breaking should be true")
	}
	if e.Title != "Breaking change" {
		t.Errorf("Title: got %q", e.Title)
	}
	if e.ReleaseNote == nil || e.ReleaseNote.Title != "release note text" {
		t.Errorf("ReleaseNote.Title: got %v", e.ReleaseNote)
	}
	if e.ReleaseNote.Description != nil {
		t.Errorf("ReleaseNote.Description should be nil for single-line note, got %v", e.ReleaseNote.Description)
	}
	if len(e.Notices) != 1 || e.Notices[0].Level != "warning" {
		t.Errorf("Notices: got %v", e.Notices)
	}
	if e.PR != nil {
		t.Errorf("PR should be nil when EnrichPR=false and no git")
	}
}

func TestBuildData_ReleaseNoteDescription(t *testing.T) {
	ch := []*changes.Change{{
		Kind:        "added",
		Bump:        "minor",
		Title:       "New feature",
		ReleaseNote: "User-facing title\nLine one.\nLine two.",
		Filename:    "feature.md",
		Notices:     []changes.Notice{},
	}}
	data := BuildData(BuildParams{Tag: "1.0.0", Changes: ch, EnrichPR: false})
	e := data.Changes[0]
	if e.ReleaseNote == nil {
		t.Fatal("expected ReleaseNote to be set")
	}
	if e.ReleaseNote.Title != "User-facing title" {
		t.Errorf("Title: got %q", e.ReleaseNote.Title)
	}
	if len(e.ReleaseNote.Description) != 2 {
		t.Errorf("Description: got %v", e.ReleaseNote.Description)
	}
	if e.ReleaseNote.Description[0] != "Line one." {
		t.Errorf("Description[0]: got %q", e.ReleaseNote.Description[0])
	}
}

func TestBuildData_MandatoryPropagates(t *testing.T) {
	ch := []*changes.Change{
		{Kind: "added", Bump: "minor", Title: "Normal", Filename: "a.md", Notices: []changes.Notice{}},
		{Kind: "changed", Bump: "minor", Title: "Must visit", Filename: "b.md", Mandatory: true, Notices: []changes.Notice{}},
	}
	data := BuildData(BuildParams{Tag: "1.0.0", Changes: ch, EnrichPR: false})
	if !data.Mandatory {
		t.Error("release Mandatory should be true when any change has Mandatory=true")
	}
	if !data.Changes[1].Mandatory {
		t.Error("change entry Mandatory should be true")
	}
}

func TestBuildData_Repository(t *testing.T) {
	ch := []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix", Filename: "f.md", Notices: []changes.Notice{}}}
	data := BuildData(BuildParams{
		Tag:        "1.0.0",
		Changes:    ch,
		GithubRepo: "OpenShock/Firmware",
		EnrichPR:   false,
	})
	if data.Repository == nil {
		t.Fatal("expected Repository to be set")
	}
	if data.Repository.Platform != "github" || data.Repository.Owner != "OpenShock" || data.Repository.Repo != "Firmware" {
		t.Errorf("Repository: got %+v", data.Repository)
	}
}

func TestBuildData_PRVerbatim(t *testing.T) {
	pr := 99
	data := BuildData(BuildParams{
		Tag:      "1.0.0",
		Changes:  []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix", Filename: "f.md", PR: &pr, Notices: []changes.Notice{}}},
		EnrichPR: false,
	})
	if data.Changes[0].PR == nil || *data.Changes[0].PR != 99 {
		t.Errorf("expected verbatim PR=99, got %v", data.Changes[0].PR)
	}
}

func TestBuildData_PRExplicitNone(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:      "1.0.0",
		Changes:  []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix", Filename: "f.md", PRExplicitNone: true, Notices: []changes.Notice{}}},
		EnrichPR: true,
	})
	if data.Changes[0].PR != nil {
		t.Errorf("expected PR=nil for explicit-none, got %v", *data.Changes[0].PR)
	}
}

func TestBuildData_NoPreviousVersion(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:      "0.1.0",
		Previous: "",
		Changes:  []*changes.Change{{Kind: "added", Bump: "minor", Title: "First feature", Filename: "first.md", Notices: []changes.Notice{}}},
		EnrichPR: false,
	})
	if data.PreviousVersion != nil {
		t.Errorf("PreviousVersion should be nil, got %q", *data.PreviousVersion)
	}
}

func TestBuildData_EmptyHeadline(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:      "1.0.0",
		Headline: "",
		Changes:  []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix", Filename: "fix.md", Notices: []changes.Notice{}}},
		EnrichPR: false,
	})
	if data.Headline != "" {
		t.Errorf("Headline should be empty string, got %q", data.Headline)
	}
}

func TestBuildData_Prerelease(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:        "1.0.0-rc.1",
		Version:    "1.0.0",
		Prerelease: true,
		Changes:    []*changes.Change{{Kind: "added", Bump: "minor", Title: "RC feature", Filename: "f.md", Notices: []changes.Notice{}}},
		EnrichPR:   false,
	})
	if !data.Prerelease {
		t.Error("expected Prerelease=true")
	}
	if data.Tag != "1.0.0-rc.1" {
		t.Errorf("Tag: got %q", data.Tag)
	}
	if data.Version != "1.0.0" {
		t.Errorf("Version: got %q", data.Version)
	}
}

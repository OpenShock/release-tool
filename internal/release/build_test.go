package release

import (
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func makeChange(bump, title string, breaking bool, categories []string) *changes.Change {
	return &changes.Change{
		Bump:        bump,
		Title:       title,
		Body:        "body text",
		ReleaseNote: "release note text",
		Breaking:    breaking,
		Categories:  categories,
		Filename:    title + ".md",
		Notices:     []changes.Notice{{Level: "warning", Message: "heads up"}},
	}
}

func TestBuildData_Fields(t *testing.T) {
	ch := []*changes.Change{
		makeChange("minor", "Add feature", false, []string{"api"}),
	}
	p := BuildParams{
		Tag:        "1.3.0",
		Previous:   "1.2.0",
		Changes:    ch,
		Headline:   "Big release",
		Prerelease: false,
		Commit:     "abc123",
		Version:    "1.3.0",
		EnrichPR:   false, // no git available in tests
	}
	data := BuildData(p)

	if data.SchemaVersion != schemaVersion {
		t.Errorf("SchemaVersion: got %d", data.SchemaVersion)
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
	if data.Headline == nil || data.Headline.Text != "Big release" {
		t.Errorf("Headline: got %v", data.Headline)
	}
	if data.ReleasedAt == "" {
		t.Error("ReleasedAt should not be empty")
	}
}

func TestBuildData_Changes(t *testing.T) {
	ch := []*changes.Change{
		makeChange("major", "Breaking change", true, []string{"core"}),
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
	if e.Type != "major" {
		t.Errorf("Type: got %q", e.Type)
	}
	if !e.Breaking {
		t.Error("Breaking should be true")
	}
	if len(e.Categories) != 1 || e.Categories[0] != "core" {
		t.Errorf("Categories: got %v", e.Categories)
	}
	if e.Title.Text != "Breaking change" {
		t.Errorf("Title.Text: got %q", e.Title.Text)
	}
	if e.Body == nil || e.Body.Text != "body text" {
		t.Errorf("Body: got %v", e.Body)
	}
	if e.ReleaseNote == nil || e.ReleaseNote.Text != "release note text" {
		t.Errorf("ReleaseNote: got %v", e.ReleaseNote)
	}
	if len(e.Notices) != 1 || e.Notices[0].Level != "warning" {
		t.Errorf("Notices: got %v", e.Notices)
	}
	if e.PR != nil {
		t.Errorf("PR should be nil when EnrichPR=false and no git")
	}
}

func TestBuildData_PRVerbatim(t *testing.T) {
	pr := 99
	data := BuildData(BuildParams{
		Tag:      "1.0.0",
		Changes:  []*changes.Change{{Bump: "patch", Title: "Fix", Filename: "f.md", Categories: []string{}, PR: &pr}},
		EnrichPR: false, // derivation off; verbatim PR must still appear
	})
	if data.Changes[0].PR == nil || *data.Changes[0].PR != 99 {
		t.Errorf("expected verbatim PR=99, got %v", data.Changes[0].PR)
	}
}

func TestBuildData_PRExplicitNone(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:      "1.0.0",
		Changes:  []*changes.Change{{Bump: "patch", Title: "Fix", Filename: "f.md", Categories: []string{}, PRExplicitNone: true}},
		EnrichPR: true, // derivation on, but explicit-none must suppress it
	})
	if data.Changes[0].PR != nil {
		t.Errorf("expected PR=nil for explicit-none, got %v", *data.Changes[0].PR)
	}
}

func TestBuildData_NoPreviousVersion(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:      "0.1.0",
		Previous: "", // bootstrap - no previous tag
		Changes:  []*changes.Change{{Bump: "minor", Title: "First feature", Filename: "first.md", Categories: []string{}}},
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
		Changes:  []*changes.Change{{Bump: "patch", Title: "Fix", Filename: "fix.md", Categories: []string{}}},
		EnrichPR: false,
	})
	if data.Headline != nil {
		t.Errorf("Headline should be nil for empty string, got %v", data.Headline)
	}
}

func TestBuildData_Prerelease(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:        "1.0.0-rc.1",
		Version:    "1.0.0",
		Prerelease: true,
		Changes:    []*changes.Change{{Bump: "minor", Title: "RC feature", Filename: "f.md", Categories: []string{}}},
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

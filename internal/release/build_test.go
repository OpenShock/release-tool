package release

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func TestWriteJSON_RoundTrip(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:        "v1.3.0",
		Previous:   "1.2.0",
		Version:    "1.3.0",
		Commit:     "deadbeef",
		Changes:    []*changes.Change{makeChange("added", "Add feature", false)},
		EnrichPR:   false,
		GithubRepo: "OpenShock/Firmware",
	})

	path := filepath.Join(t.TempDir(), "release.json")
	if err := WriteJSON(path, data); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got ReleaseData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.SchemaVersion != schemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", got.SchemaVersion, schemaVersion)
	}
	if got.Tag != "v1.3.0" || got.Version != "1.3.0" || got.Commit != "deadbeef" {
		t.Errorf("core fields not preserved: %+v", got)
	}
	if got.Repository == nil || got.Repository.Owner != "OpenShock" || got.Repository.Repo != "Firmware" {
		t.Errorf("Repository not preserved: %+v", got.Repository)
	}
	if len(got.Changes) != 1 || got.Changes[0].Title != "Add feature" {
		t.Errorf("Changes not preserved: %+v", got.Changes)
	}

	// omitempty: no previous tag, headline, or PR set -> keys must be absent.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"previous_tag", "headline"} {
		if _, present := m[k]; present {
			t.Errorf("%q should be omitted when unset", k)
		}
	}
	if _, present := m["previous_version"]; !present {
		t.Error("previous_version should be present when Previous is set")
	}
}

func TestBuildData_TitlePRSuffixMovesToPR(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:     "1.0.0",
		Changes: []*changes.Change{{Kind: "added", Bump: "minor", Title: "Add wizard (#426)", Filename: "w.md", Notices: []changes.Notice{}}},
	})
	e := data.Changes[0]
	if e.Title != "Add wizard" {
		t.Errorf("title suffix should be stripped, got %q", e.Title)
	}
	if e.PR == nil || *e.PR != 426 {
		t.Errorf("expected PR=426 from title, got %v", e.PR)
	}
}

func TestBuildData_TitlePRSuffixMatchingExplicitPR(t *testing.T) {
	pr := 7
	data := BuildData(BuildParams{
		Tag:     "1.0.0",
		Changes: []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix (#7)", Filename: "f.md", PR: &pr, Notices: []changes.Notice{}}},
	})
	if got := data.Changes[0].Title; got != "Fix" {
		t.Errorf("duplicate (#7) should be stripped, got %q", got)
	}
}

func TestBuildData_TitlePRSuffixKeptForExplicitNone(t *testing.T) {
	data := BuildData(BuildParams{
		Tag:     "1.0.0",
		Changes: []*changes.Change{{Kind: "fixed", Bump: "patch", Title: "Fix (#7)", Filename: "f.md", PRExplicitNone: true, Notices: []changes.Notice{}}},
	})
	e := data.Changes[0]
	if e.Title != "Fix (#7)" || e.PR != nil {
		t.Errorf("explicit pr: null should keep the title as authored, got %q pr=%v", e.Title, e.PR)
	}
}

func TestBuildData_BreakingFirst(t *testing.T) {
	data := BuildData(BuildParams{
		Tag: "2.0.0",
		Changes: []*changes.Change{
			makeChange("changed", "a-normal", false),
			makeChange("changed", "b-breaking", true),
		},
	})
	if data.Changes[0].Title != "b-breaking" {
		t.Errorf("breaking change should sort first, got %q", data.Changes[0].Title)
	}
}

// gitRepoWithChanges commits each name under .changes/ with the given subject at
// the given unix time (in that order) and returns the repo root.
func gitRepoWithChanges(t *testing.T, files []struct {
	name, subject string
	at            int64
}) string {
	t.Helper()
	root := t.TempDir()
	runGit := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1"), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit(nil, "init", "-b", "master")
	runGit(nil, "config", "user.name", "test")
	runGit(nil, "config", "user.email", "test@test.com")
	runGit(nil, "config", "commit.gpgsign", "false")
	if err := os.MkdirAll(filepath.Join(root, ".changes"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, ".changes", f.name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		date := fmt.Sprintf("@%d +0000", f.at)
		runGit(nil, "add", ".changes/"+f.name)
		runGit([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}, "commit", "-m", f.subject)
	}
	return root
}

func TestBuildData_OrderedByCommitTimeAndPRFromSubject(t *testing.T) {
	root := gitRepoWithChanges(t, []struct {
		name, subject string
		at            int64
	}{
		{"zeta.md", "fix: older change (#12)", 1_700_000_000},
		{"alpha.md", "Merge pull request #34 from someone/branch", 1_700_000_100},
	})
	mk := func(file string) *changes.Change {
		return &changes.Change{Kind: "fixed", Bump: "patch", Title: file, Filename: file, Notices: []changes.Notice{}}
	}
	// Filename order (alpha before zeta) and uncommitted last.
	data := BuildData(BuildParams{
		Tag:     "1.0.1",
		Root:    root,
		Changes: []*changes.Change{mk("alpha.md"), mk("new.md"), mk("zeta.md")},
	})

	order :=[]string{data.Changes[0].Title, data.Changes[1].Title, data.Changes[2].Title}
	want := []string{"zeta.md", "alpha.md", "new.md"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order: got %v, want %v", order, want)
		}
	}
	if pr := data.Changes[0].PR; pr == nil || *pr != 12 {
		t.Errorf("squash subject PR: got %v, want 12", pr)
	}
	if pr := data.Changes[1].PR; pr == nil || *pr != 34 {
		t.Errorf("merge subject PR: got %v, want 34", pr)
	}
	if data.Changes[2].PR != nil {
		t.Errorf("uncommitted file should have no PR, got %v", *data.Changes[2].PR)
	}
}

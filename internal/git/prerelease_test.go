package git

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPreviousPrereleaseTag(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "1")
	if err := CreateTag(root, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got := PreviousPrereleaseTag(root, "v", "rc", "v1.0.0", ""); got != "" {
		t.Errorf("no prerelease yet, got %q", got)
	}
	commitFile(t, root, "b.txt", "2")
	for _, tag := range []string{"v1.1.0-rc.2", "v1.1.0-rc.10", "v1.1.0-beta.1"} {
		if err := CreateTag(root, tag); err != nil {
			t.Fatal(err)
		}
	}
	if got := PreviousPrereleaseTag(root, "v", "rc", "v1.0.0", ""); got != "v1.1.0-rc.10" {
		t.Errorf("highest rc should win (numeric, label-filtered), got %q", got)
	}
	if got := PreviousPrereleaseTag(root, "v", "rc", "v1.0.0", "v1.1.0-rc.10"); got != "v1.1.0-rc.2" {
		t.Errorf("exclude should skip the tag being created, got %q", got)
	}

	// A prerelease that predates the latest stable belongs to an older cycle.
	commitFile(t, root, "c.txt", "3")
	if err := CreateTag(root, "v1.1.0"); err != nil {
		t.Fatal(err)
	}
	if got := PreviousPrereleaseTag(root, "v", "rc", "v1.1.0", ""); got != "" {
		t.Errorf("rc tags before the stable must be ignored, got %q", got)
	}
}

func TestChangedChangeFilesSinceRef_IgnoresArchive(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "seed.txt", "x")
	base, err := CurrentCommit(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".changes", "released", "1.0.0")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	commitFile(t, root, ".changes/new.md", "y")
	commitFile(t, root, ".changes/_known-issues.md", "- z")
	if err := Add(root, ".changes/released"); err != nil {
		t.Fatal(err)
	}
	if err := Commit(root, "archive"); err != nil {
		t.Fatal(err)
	}
	got, err := ChangedChangeFilesSinceRef(root, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"new.md"}) {
		t.Errorf("only top-level change files count, got %v", got)
	}
}

func TestChangedFilesSinceRef(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "1")
	base, err := CurrentCommit(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	commitFile(t, root, "docs/b.md", "2")
	got, err := ChangedFilesSinceRef(root, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"docs/b.md"}) {
		t.Errorf("got %v", got)
	}
}

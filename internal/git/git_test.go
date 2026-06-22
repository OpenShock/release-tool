package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRepo initialises a hermetic git repo with a local identity, so the package
// functions (which inherit the ambient environment) commit deterministically
// regardless of the developer's global git config.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-b", "master")
	runGit("config", "user.name", "test")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "commit.gpgsign", "false")
	runGit("config", "tag.gpgsign", "false")
	return root
}

// commitFile writes name with content, stages it via Add, and commits via Commit.
func commitFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Add(root, name); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Commit(root, "add "+name); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestAddCommit(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "hello")

	tracked, err := run(root, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if tracked != "a.txt" {
		t.Errorf("expected a.txt to be tracked, got %q", tracked)
	}
}

func TestCreateTagAndTagExists(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "hello")

	if exists, err := TagExists(root, "v1.0.0"); err != nil || exists {
		t.Fatalf("tag should not exist yet (exists=%v err=%v)", exists, err)
	}
	if err := CreateTag(root, "v1.0.0"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if exists, err := TagExists(root, "v1.0.0"); err != nil || !exists {
		t.Fatalf("tag should exist now (exists=%v err=%v)", exists, err)
	}

	// The tag is annotated.
	typ, err := run(root, "cat-file", "-t", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if typ != "tag" {
		t.Errorf("expected an annotated tag object, got %q", typ)
	}
}

func TestCreateTag_RejectsUnsafeName(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "hello")

	for _, bad := range []string{"-rf", "--force", ""} {
		if err := CreateTag(root, bad); err == nil {
			t.Errorf("CreateTag(%q) should be rejected", bad)
		}
	}
}

func TestResetHard_RestoresWorkingTree(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "keep.txt", "v1")
	head, err := CurrentCommit(root)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate a tracked file and add+commit a new one, then roll back to head.
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	commitFile(t, root, "extra.txt", "should vanish")

	if err := ResetHard(root, head); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "keep.txt"))
	if err != nil || string(got) != "v1" {
		t.Errorf("keep.txt should be restored to v1, got %q (err=%v)", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "extra.txt")); !os.IsNotExist(err) {
		t.Errorf("extra.txt (committed after head) should be gone after reset")
	}
	if cur, _ := CurrentCommit(root); cur != head {
		t.Errorf("HEAD should be back at %s, got %s", head, cur)
	}
}

func TestLatestStableTag(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "hello")
	for _, tag := range []string{"v1.0.0", "v1.2.0", "v1.10.0", "v1.2.0-rc.1"} {
		if err := CreateTag(root, tag); err != nil {
			t.Fatal(err)
		}
	}

	got, err := LatestStableTag(root, "v")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.10.0" {
		t.Errorf("expected 1.10.0 (numeric sort, prerelease ignored), got %q", got)
	}
}

func TestLatestPrereleaseNumber(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "a.txt", "hello")
	for _, tag := range []string{"v1.2.0-rc.1", "v1.2.0-rc.3", "v1.2.0-beta.5"} {
		if err := CreateTag(root, tag); err != nil {
			t.Fatal(err)
		}
	}

	n, err := LatestPrereleaseNumber(root, "v1.2.0", "rc")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected highest rc number 3, got %d", n)
	}

	// A label with no tags yet starts the count at 0.
	if n, err := LatestPrereleaseNumber(root, "v1.2.0", "alpha"); err != nil || n != 0 {
		t.Errorf("expected 0 for an unused label, got %d (err=%v)", n, err)
	}
}

func TestChangedChangeFilesSinceRef_DedupesAndFilters(t *testing.T) {
	root := newRepo(t)
	commitFile(t, root, "seed.txt", "seed")
	base, err := CurrentCommit(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(root, ".changes"), 0755); err != nil {
		t.Fatal(err)
	}
	commitFile(t, root, filepath.Join(".changes", "one.md"), "---\nkind: fixed\n---\nOne\n")
	commitFile(t, root, filepath.Join(".changes", "README.md"), "# readme")
	// Remove and re-add one.md so it appears as "added" twice across history.
	if err := os.Remove(filepath.Join(root, ".changes", "one.md")); err != nil {
		t.Fatal(err)
	}
	if err := Add(root, filepath.Join(".changes", "one.md")); err != nil {
		t.Fatal(err)
	}
	if err := Commit(root, "remove one.md"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, root, filepath.Join(".changes", "one.md"), "---\nkind: fixed\n---\nOne again\n")

	files, err := ChangedChangeFilesSinceRef(root, base)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, f := range files {
		count[f]++
	}
	if count["one.md"] != 1 {
		t.Errorf("one.md should appear exactly once despite re-add, got %d (%v)", count["one.md"], files)
	}
	if count["README.md"] != 0 {
		t.Errorf("README.md should be filtered out, got %v", files)
	}
}

func TestIdentityConfigured(t *testing.T) {
	root := newRepo(t)
	if !IdentityConfigured(root) {
		t.Error("repo with local user.name/user.email should report identity configured")
	}
}

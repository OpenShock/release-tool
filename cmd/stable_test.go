package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// releaseRepo builds a committed repo with the given tag prefix, a stable-release
// branch config, and one valid pending change file.
func releaseRepo(t *testing.T, tagPrefix string) string {
	t.Helper()
	root := t.TempDir()
	gitCmd(t, root, "init", "-b", "master")
	gitCmd(t, root, "config", "user.name", "test")
	gitCmd(t, root, "config", "user.email", "test@test.com")
	gitCmd(t, root, "config", "commit.gpgsign", "false")
	gitCmd(t, root, "config", "tag.gpgsign", "false")
	mustWrite(t, filepath.Join(root, ".changes", "config.json"),
		`{"tag_prefix":`+strconv.Quote(tagPrefix)+`,"branches":{"master":{"release":"stable"}}}`)
	mustWrite(t, filepath.Join(root, ".changes", "feat.md"), "---\nkind: added\n---\nAdd a feature\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-m", "seed")
	return root
}

// TestRunRelease_RollsBackOnTagFailure exercises the destructive release path:
// an unsafe tag prefix passes the pre-flight checks but makes CreateTag reject
// the composed tag *after* the CHANGELOG was written, the change file deleted,
// and the release commit created. The rollback must restore the pre-release
// state so the release isn't silently lost on a re-run.
func TestRunRelease_RollsBackOnTagFailure(t *testing.T) {
	root := releaseRepo(t, "-bad-") // leading dash => CreateTag rejects "-bad-0.1.0"

	prev := rootDir
	rootDir = root
	t.Cleanup(func() { rootDir = prev })

	head := gitCmd(t, root, "rev-parse", "HEAD")

	err := runRelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json")})
	if err == nil {
		t.Fatal("expected runRelease to fail when CreateTag rejects the unsafe tag")
	}

	if cur := gitCmd(t, root, "rev-parse", "HEAD"); cur != head {
		t.Errorf("HEAD moved: the release commit was not rolled back (%s != %s)", cur, head)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes", "feat.md")); err != nil {
		t.Errorf("change file must be restored after rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CHANGELOG.md")); !os.IsNotExist(err) {
		t.Errorf("CHANGELOG.md should have been rolled back, but it exists")
	}
	if tags := gitCmd(t, root, "tag"); tags != "" {
		t.Errorf("no tag should exist after rollback, got %q", tags)
	}
	if st := gitCmd(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("working tree should be clean after rollback, got:\n%s", st)
	}
}

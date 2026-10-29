package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenShock/release-tool/internal/release"
)

func useRoot(t *testing.T, root string) {
	t.Helper()
	prev := rootDir
	rootDir = root
	t.Cleanup(func() { rootDir = prev })
}

// TestRunRelease_ArchivesAndMarksYanked covers a successful stable release:
// consumed change files and the headline move to .changes/released/<version>/,
// known issues stay, and a yanked version's CHANGELOG heading gets [YANKED].
func TestRunRelease_ArchivesAndMarksYanked(t *testing.T) {
	root := releaseRepo(t, "v")
	mustWrite(t, filepath.Join(root, ".changes", "pull.md"), "---\nkind: fixed\nyanks: [0.0.1]\n---\nFix the broken release\n")
	mustWrite(t, filepath.Join(root, ".changes", "_headline.md"), "Headline.\n")
	mustWrite(t, filepath.Join(root, ".changes", "_known-issues.md"), "- Still broken on Tuesdays.\n")
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"), "## [v0.0.1] - 2026-01-01\n\n### Added\n- First\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-m", "pending")
	useRoot(t, root)

	notes := filepath.Join(t.TempDir(), "notes.md")
	if err := runRelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json"), notes: notes}); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"feat.md", "pull.md", "_headline.md"} {
		if _, err := os.Stat(filepath.Join(root, ".changes", f)); !os.IsNotExist(err) {
			t.Errorf(".changes/%s should have been moved out", f)
		}
		if _, err := os.Stat(filepath.Join(root, ".changes", "released", "0.1.0", f)); err != nil {
			t.Errorf(".changes/released/0.1.0/%s should exist: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".changes", "_known-issues.md")); err != nil {
		t.Errorf("known issues must survive a release: %v", err)
	}
	if st := gitCmd(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("release commit should include every move, got:\n%s", st)
	}

	changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "## [v0.0.1] - 2026-01-01 [YANKED]") {
		t.Errorf("yanked version not marked:\n%s", changelog)
	}
	body, err := os.ReadFile(notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"**Version 0.0.1 has been withdrawn.**", "### Known issues\n- Still broken on Tuesdays."} {
		if !strings.Contains(string(body), want) {
			t.Errorf("notes missing %q:\n%s", want, body)
		}
	}
}

func TestRunRelease_FailsOnLintError(t *testing.T) {
	root := releaseRepo(t, "v")
	mustWrite(t, filepath.Join(root, ".changes", "drop.md"), "---\nkind: removed\nbreaking: true\n---\nRemove the API\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-m", "breaking")
	useRoot(t, root)

	head := gitCmd(t, root, "rev-parse", "HEAD")
	if err := runRelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json")}); err == nil {
		t.Fatal("a breaking change without guidance must block the release")
	}
	if cur := gitCmd(t, root, "rev-parse", "HEAD"); cur != head {
		t.Error("nothing should be committed when lint fails")
	}
}

// TestRunPrerelease_DeltaSincePreviousRC checks that rc.2 records which
// changes are new since rc.1.
func TestRunPrerelease_DeltaSincePreviousRC(t *testing.T) {
	root := releaseRepo(t, "v")
	gitCmd(t, root, "tag", "-a", "-m", "v0.1.0-rc.1", "v0.1.0-rc.1")
	mustWrite(t, filepath.Join(root, ".changes", "second.md"), "---\nkind: fixed\n---\nFix the second thing\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-m", "second")
	useRoot(t, root)

	out := filepath.Join(t.TempDir(), "release.json")
	if err := runPrerelease(releaseOptions{output: out, prereleaseLabel: "rc"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var data release.ReleaseData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Tag != "v0.1.0-rc.2" {
		t.Errorf("tag = %q", data.Tag)
	}
	if d := data.SincePrerelease; d == nil || d.Tag != "v0.1.0-rc.1" || !reflect.DeepEqual(d.Changes, []string{"second"}) {
		t.Errorf("SincePrerelease = %+v", d)
	}
	if len(data.Changes) != 2 {
		t.Errorf("prerelease notes stay cumulative since the last stable, got %d changes", len(data.Changes))
	}
}

// versionFilesRepo is releaseRepo with frontend/package.json listed under
// version_files.
func versionFilesRepo(t *testing.T) string {
	t.Helper()
	root := releaseRepo(t, "v")
	mustWrite(t, filepath.Join(root, ".changes", "config.json"),
		`{"tag_prefix":"v","branches":{"master":{"release":"stable"}},"version_files":[{"path":"frontend/package.json","json_key":"version"}]}`)
	mustWrite(t, filepath.Join(root, "frontend", "package.json"), "{\n\t\"name\": \"frontend\",\n\t\"version\": \"0.0.0\"\n}\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-m", "version files")
	useRoot(t, root)
	return root
}

func readVersion(t *testing.T, root string) string {
	t.Helper()
	var pkg struct{ Version string }
	raw, err := os.ReadFile(filepath.Join(root, "frontend", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	return pkg.Version
}

func TestRunRelease_SetsVersionFiles(t *testing.T) {
	root := versionFilesRepo(t)
	if err := runRelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json")}); err != nil {
		t.Fatal(err)
	}
	if v := readVersion(t, root); v != "0.1.0" {
		t.Errorf("package.json version = %q, want 0.1.0 (no tag prefix)", v)
	}
	if st := gitCmd(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("version file should be in the release commit, got:\n%s", st)
	}
	if files := gitCmd(t, root, "show", "--name-only", "--format=", "v0.1.0"); !strings.Contains(files, "frontend/package.json") {
		t.Errorf("tagged release commit does not touch package.json:\n%s", files)
	}
}

func TestRunPrerelease_CommitsVersionFiles(t *testing.T) {
	root := versionFilesRepo(t)
	start := gitCmd(t, root, "rev-parse", "HEAD")
	if err := runPrerelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json"), prereleaseLabel: "rc"}); err != nil {
		t.Fatal(err)
	}
	if v := readVersion(t, root); v != "0.1.0-rc.1" {
		t.Errorf("package.json version = %q, want 0.1.0-rc.1", v)
	}
	if subj := gitCmd(t, root, "log", "-1", "--format=%s"); subj != "chore: release v0.1.0-rc.1" {
		t.Errorf("release commit subject = %q", subj)
	}
	if parent := gitCmd(t, root, "rev-parse", "HEAD~1"); parent != start {
		t.Errorf("expected exactly one release commit on top of %s", start)
	}
	if tagged, head := gitCmd(t, root, "rev-parse", "v0.1.0-rc.1^{commit}"), gitCmd(t, root, "rev-parse", "HEAD"); tagged != head {
		t.Errorf("tag points at %s, want the release commit %s", tagged, head)
	}
}

func TestRunPrerelease_SHABuildLeavesVersionFiles(t *testing.T) {
	root := versionFilesRepo(t)
	start := gitCmd(t, root, "rev-parse", "HEAD")
	if err := runPrerelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json"), prereleaseLabel: "develop", gitSHA: true}); err != nil {
		t.Fatal(err)
	}
	if v := readVersion(t, root); v != "0.0.0" {
		t.Errorf("SHA build changed package.json to %q", v)
	}
	if head := gitCmd(t, root, "rev-parse", "HEAD"); head != start {
		t.Errorf("SHA build made a commit")
	}
}

func TestRunPrerelease_RollsBackVersionCommitOnTagFailure(t *testing.T) {
	root := versionFilesRepo(t)
	start := gitCmd(t, root, "rev-parse", "HEAD")
	// refs/tags/v0.1.0-rc.1 cannot be created while refs/tags/v0.1.0-rc.1/ is a
	// directory, so CreateTag fails after the release commit was made.
	gitCmd(t, root, "tag", "v0.1.0-rc.1/blocker")
	err := runPrerelease(releaseOptions{output: filepath.Join(t.TempDir(), "release.json"), prereleaseLabel: "rc"})
	if err == nil {
		t.Fatal("expected CreateTag to fail")
	}
	if head := gitCmd(t, root, "rev-parse", "HEAD"); head != start {
		t.Errorf("release commit was not rolled back")
	}
	if v := readVersion(t, root); v != "0.0.0" {
		t.Errorf("package.json not restored, version = %q", v)
	}
}

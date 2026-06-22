package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

// makeRepo creates a minimal .changes/ directory in a temp dir and returns the root.
func makeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".changes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// config.json with empty categories so any category is accepted
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"tag_prefix":"","branches":{"master":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var releaseBranchConfig = &changes.Config{
	Branches: map[string]changes.BranchConfig{"master": {}},
}

func TestRunCheck_SkipWhenNoConfig(t *testing.T) {
	v, err := RunCheck(CheckParams{BaseBranch: "master", Config: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateSkip {
		t.Errorf("expected skip with nil config, got %q", v.State)
	}
}

func TestRunCheck_SkipWhenNotReleaseBranch(t *testing.T) {
	v, err := RunCheck(CheckParams{
		BaseBranch: "some-feature-branch",
		PR:         7,
		Config:     releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateSkip {
		t.Errorf("expected skip, got %q", v.State)
	}
	if v.PR != 7 {
		t.Errorf("PR should pass through, got %d", v.PR)
	}
	if v.Body != "" {
		t.Errorf("skip should have no body, got %q", v.Body)
	}
}

// gitExec runs git in root with a hermetic identity and returns trimmed stdout.
func gitExec(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_CONFIG_NOSYSTEM=1", "HOME=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	gitExec(t, root, "init", "-b", "master")
	gitExec(t, root, "config", "commit.gpgsign", "false")
	gitExec(t, root, "commit", "--allow-empty", "-m", "init")
}

// addChangeFile writes .changes/<name> and commits it, returning nothing; the
// caller diffs against a previously captured ref.
func addChangeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".changes", name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, root, "add", filepath.Join(".changes", name))
	gitExec(t, root, "commit", "-m", "add "+name)
}

func TestRunCheck_MissingWhenNoFilesAdded(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)

	v, err := RunCheck(CheckParams{
		Root:       root,
		BaseBranch: "master",
		Against:    "HEAD",
		Config:     releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateMissing {
		t.Errorf("expected missing, got %q", v.State)
	}
}

func TestRunCheck_InvalidWhenPRFieldSet(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "my-change.md", "---\nkind: fixed\npr: 42\n---\nSome change\n")

	v, err := RunCheck(CheckParams{
		Root: root, BaseBranch: "master", Against: base, PR: 7, Config: releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateInvalid {
		t.Errorf("expected invalid, got %q", v.State)
	}
	if v.PR != 7 {
		t.Errorf("PR should pass through, got %d", v.PR)
	}
	if !strings.Contains(v.Body, "pr field must not be set") || !strings.Contains(v.Body, "my-change.md") {
		t.Errorf("body should explain the rejected pr field:\n%s", v.Body)
	}
}

func TestRunCheck_InvalidWhenFileMalformed(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "bad.md", "---\nkind: fixed\n---\nTitle\n\n## Notices\n- warming: typo level\n")

	v, err := RunCheck(CheckParams{
		Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateInvalid {
		t.Errorf("expected invalid for a malformed notice, got %q", v.State)
	}
}

func TestRunCheck_OKForValidChangeFile(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "good-change.md", "---\nkind: added\n---\nA valid change\n")

	v, err := RunCheck(CheckParams{
		Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateOK {
		t.Errorf("expected ok, got %q (body: %s)", v.State, v.Body)
	}
	if !strings.Contains(v.Body, "**OK**") {
		t.Errorf("body should report OK:\n%s", v.Body)
	}
}

func TestRunCheck_StaleBaseIgnoresBaseAccruedFiles(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root) // M0 — the PR's (soon to be stale) base
	staleBase := gitExec(t, root, "rev-parse", "HEAD")

	// The base branch accrues a new change file AFTER the PR's recorded base.
	addChangeFile(t, root, "base-added.md", "---\nkind: added\n---\nBase feature\n")

	// PR branch forks from the stale base and adds NO change file.
	gitExec(t, root, "checkout", "-q", "-b", "pr", staleBase)
	gitExec(t, root, "commit", "--allow-empty", "-m", "pr work")
	prHead := gitExec(t, root, "rev-parse", "HEAD")

	// Simulate GitHub's refs/pull/N/merge: master merged into the PR head, so
	// HEAD is a merge whose first parent is the current base tip.
	gitExec(t, root, "checkout", "-q", "master")
	gitExec(t, root, "checkout", "-q", "-b", "mergeref")
	gitExec(t, root, "merge", "--no-ff", "--no-edit", prHead)

	// With the stale base the naive diff would count base-added.md and report OK;
	// resolving to the merge-ref base parent must yield Missing.
	v, err := RunCheck(CheckParams{
		Root: root, BaseBranch: "master", Against: staleBase, Config: releaseBranchConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateMissing {
		t.Errorf("expected Missing (PR adds no change file), got %q; body:\n%s", v.State, v.Body)
	}
}

func TestRenderCheckBody(t *testing.T) {
	cases := []struct {
		state   CheckState
		count   int
		detail  string
		wantStr string
	}{
		{StateOK, 2, "", "**OK**"},
		{StateOK, 3, "", "3 valid change file"},
		{StateMissing, 0, "", "**Missing**"},
		{StateInvalid, 1, "bad.md: invalid notice level \"warming\"", "warming"},
		{StateInvalid, 1, "my.md: pr field must not be set in a PR", "pr field must not be set"},
	}
	for _, c := range cases {
		body := renderCheckBody(c.state, c.count, c.detail)
		if !strings.HasPrefix(body, checkMarker) {
			t.Errorf("%s: body must start with the sticky marker", c.state)
		}
		if !strings.Contains(body, c.wantStr) {
			t.Errorf("%s: body missing %q in:\n%s", c.state, c.wantStr, body)
		}
	}
}

func TestRenderCheckBody_FenceContainsBacktickContent(t *testing.T) {
	// Attacker-controlled content with a triple-backtick run must not be able to
	// break out of the code fence in the privileged PR comment.
	detail := "evil.md: ```\n## Injected heading\n![img](http://x)"
	body := renderCheckBody(StateInvalid, 1, detail)
	fence := safeFence(strings.TrimSpace(detail))
	if len(fence) <= 3 {
		t.Errorf("fence should be longer than the embedded backtick run, got %q", fence)
	}
	// The opening fence must be the longer fence, immediately followed by content.
	if !strings.Contains(body, fence+"\n"+strings.TrimSpace(detail)) {
		t.Errorf("content should be wrapped by a fence longer than its backtick run:\n%s", body)
	}
}

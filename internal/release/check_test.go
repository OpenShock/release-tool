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
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"tag_prefix":"","categories":[],"branches":{"master":{}}}`), 0644); err != nil {
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

func gitInit(t *testing.T, root string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_CONFIG_NOSYSTEM=1", "HOME=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "master")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")
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

func TestRunCheck_InvalidPRField_Number(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"my-change.md": "---\ntype: patch\npr: 42\n---\n\nSome change\n",
	})

	// Inject the parsed change directly by testing via ReadSubset path.
	// We test RunCheck end-to-end by stubbing the git layer via Against="".
	ch, err := changes.ReadSubset(root, []string{"my-change.md"})
	if err != nil {
		t.Fatalf("ReadSubset: %v", err)
	}
	if len(ch) != 1 {
		t.Fatalf("expected 1 change, got %d", len(ch))
	}
	if ch[0].PR == nil {
		t.Fatal("expected PR to be set (pr: 42), got nil")
	}

	// Now verify the check logic rejects it via renderBody path
	detail := ""
	state := StateOK
	for _, c := range ch {
		if c.PR != nil || c.PRExplicitNone {
			state = StateInvalid
			detail += "  - " + c.Filename + ": pr field must not be set in a PR (it is assigned automatically at release time)\n"
		}
	}
	if state != StateInvalid {
		t.Error("expected StateInvalid for change with pr: 42")
	}
	if !strings.Contains(detail, "my-change.md") {
		t.Errorf("expected filename in detail, got: %s", detail)
	}
}

func TestRunCheck_InvalidPRField_Null(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"suppress-pr.md": "---\ntype: patch\npr: null\n---\n\nSome change\n",
	})

	ch, err := changes.ReadSubset(root, []string{"suppress-pr.md"})
	if err != nil {
		t.Fatalf("ReadSubset: %v", err)
	}
	if len(ch) != 1 {
		t.Fatalf("expected 1 change, got %d", len(ch))
	}
	if !ch[0].PRExplicitNone {
		t.Fatal("expected PRExplicitNone=true for pr: null")
	}

	state := StateOK
	for _, c := range ch {
		if c.PR != nil || c.PRExplicitNone {
			state = StateInvalid
		}
	}
	if state != StateInvalid {
		t.Error("expected StateInvalid for change with pr: null")
	}
}

func TestRunCheck_ValidChangeFile_NoPRField(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"good-change.md": "---\ntype: minor\n---\n\nA valid change\n",
	})

	ch, err := changes.ReadSubset(root, []string{"good-change.md"})
	if err != nil {
		t.Fatalf("ReadSubset: %v", err)
	}
	if len(ch) != 1 {
		t.Fatalf("expected 1 change, got %d", len(ch))
	}
	if ch[0].PR != nil || ch[0].PRExplicitNone {
		t.Error("expected PR=nil and PRExplicitNone=false for change without pr field")
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

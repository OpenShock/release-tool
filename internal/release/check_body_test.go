package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func TestRenderCheckBody(t *testing.T) {
	finding := func(file, msg string) []changes.Finding {
		return []changes.Finding{{File: file, Severity: changes.SeverityError, Message: msg}}
	}
	cases := []struct {
		state    CheckState
		count    int
		findings []changes.Finding
		wantStr  string
	}{
		{StateOK, 2, nil, "**OK**"},
		{StateOK, 3, nil, "3 valid change file"},
		{StateMissing, 0, nil, "**Missing**"},
		{StateMissing, 0, nil, "no-changelog"},
		{StateInvalid, 1, finding("bad.md", `invalid notice level "warming"`), "warming"},
		{StateInvalid, 1, finding("my.md", "pr field must not be set in a PR"), "pr field must not be set"},
		{StateOK, 1, []changes.Finding{{File: "x.md", Severity: changes.SeverityWarning, Message: "use the imperative"}}, "Suggestions"},
	}
	for _, c := range cases {
		body := renderCheckBody(c.state, c.count, c.findings)
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
	f := changes.Finding{File: "evil.md", Severity: changes.SeverityError, Message: "```\n## Injected heading\n![img](http://x)"}
	body := renderCheckBody(StateInvalid, 1, []changes.Finding{f})
	fence := safeFence(f.String())
	if len(fence) <= 3 {
		t.Errorf("fence should be longer than the embedded backtick run, got %q", fence)
	}
	// The opening fence must be the longer fence, immediately followed by content.
	if !strings.Contains(body, fence+"\n"+f.String()) {
		t.Errorf("content should be wrapped by a fence longer than its backtick run:\n%s", body)
	}
}

func TestRunCheck_InvalidWhenBreakingWithoutGuidance(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "drop-api.md", "---\nkind: removed\nbreaking: true\n---\nRemove the old API\n")

	v, err := RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateInvalid {
		t.Errorf("expected invalid for a breaking change with no guidance, got %q", v.State)
	}
	if len(v.Findings) != 1 || v.Findings[0].File != "drop-api.md" {
		t.Errorf("expected one finding on drop-api.md, got %+v", v.Findings)
	}
}

func TestRunCheck_WarningsKeepOK(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "past.md", "---\nkind: fixed\n---\nFixed a crash\n")

	v, err := RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateOK {
		t.Errorf("warnings must not fail the check, got %q", v.State)
	}
	if len(v.Findings) != 1 || v.Findings[0].Severity != changes.SeverityWarning {
		t.Errorf("expected one warning, got %+v", v.Findings)
	}
}

func TestRunCheck_SkipWhenOnlyIgnoredPathsChange(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	if err := os.MkdirAll(filepath.Join(root, "docs", "guide"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide", "a.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, root, "add", "docs")
	gitExec(t, root, "commit", "-q", "-m", "docs")

	cfg := &changes.Config{Branches: releaseBranchConfig.Branches, IgnorePaths: []string{"docs/**"}}
	v, err := RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateSkip {
		t.Errorf("expected skip for a docs-only PR, got %q", v.State)
	}

	cfg.IgnorePaths = []string{"*.md"}
	v, err = RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateMissing {
		t.Errorf("a single * must not cross directories; expected missing, got %q", v.State)
	}
}

func TestGlobRe(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"docs/**", "docs/a/b.md", true},
		{"docs/**", "src/docs/a.md", false},
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "c.md", true},
		{"*.md", "a/c.md", false},
		{".github/*", ".github/x.yml", true},
		{"a?c", "abc", true},
		{"a.c", "abc", false},
	}
	for _, c := range cases {
		if got := globRe(c.glob).MatchString(c.path); got != c.want {
			t.Errorf("globRe(%q).Match(%q) = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

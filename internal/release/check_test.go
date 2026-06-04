package release

import (
	"strings"
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func TestRunCheck_SkipWhenNotReleaseBranch(t *testing.T) {
	// base branch is absent from Branches, so RunCheck returns before touching git.
	v, err := RunCheck(CheckParams{
		BaseBranch: "some-feature-branch",
		PR:         7,
		Config:     &changes.Config{Branches: map[string]changes.BranchConfig{"master": {}}},
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

func TestRunCheck_SkipWhenNoConfig(t *testing.T) {
	v, err := RunCheck(CheckParams{BaseBranch: "master", Config: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.State != StateSkip {
		t.Errorf("expected skip with nil config, got %q", v.State)
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
		{StateMissing, 0, "", "**Missing**"},
		{StateInvalid, 1, "bad.md: invalid notice level \"warming\"", "warming"},
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

func TestRenderCheckBody_OKCount(t *testing.T) {
	body := renderCheckBody(StateOK, 3, "")
	if !strings.Contains(body, "3 valid change file") {
		t.Errorf("expected count in body:\n%s", body)
	}
}

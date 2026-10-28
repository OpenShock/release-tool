package release

import (
	"strings"
	"testing"
)

func TestRunCheck_ReportsVersionImpact(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	gitExec(t, root, "tag", "1.2.0")
	addChangeFile(t, root, "base-fix.md", "---\nkind: fixed\n---\nFix something\n")
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "pr-feature.md", "---\nkind: added\n---\nAdd something\n")

	v, err := RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig})
	if err != nil {
		t.Fatal(err)
	}
	want := VersionImpact{Bump: "minor", Latest: "1.2.0", Before: "1.2.1", After: "1.3.0"}
	if v.Impact == nil || *v.Impact != want {
		t.Fatalf("Impact = %+v, want %+v", v.Impact, want)
	}
	if !strings.Contains(v.Body, "**Version impact:** **minor** bump (new or changed behaviour). The next release becomes **1.3.0**, up from 1.2.1 without this PR (latest stable: 1.2.0).") {
		t.Errorf("body missing the version line:\n%s", v.Body)
	}
}

func TestRunCheck_NoImpactWhenInvalid(t *testing.T) {
	root := makeRepo(t, nil)
	gitInit(t, root)
	base := gitExec(t, root, "rev-parse", "HEAD")
	addChangeFile(t, root, "bad.md", "---\nkind: nope\n---\nT\n")

	v, err := RunCheck(CheckParams{Root: root, BaseBranch: "master", Against: base, Config: releaseBranchConfig})
	if err != nil {
		t.Fatal(err)
	}
	if v.Impact != nil || strings.Contains(v.Body, "Version impact") {
		t.Errorf("an invalid PR should report no version impact: %+v\n%s", v.Impact, v.Body)
	}
}

func TestWriteImpact(t *testing.T) {
	cases := []struct {
		im   VersionImpact
		want string
	}{
		{VersionImpact{Bump: "patch", Latest: "1.0.0", After: "1.0.1"}, "**patch** bump (fix). The next release will be **1.0.1** (latest stable: 1.0.0)."},
		{VersionImpact{Bump: "patch", Latest: "1.0.0", Before: "1.1.0", After: "1.1.0"}, "The next release stays **1.1.0** (latest stable: 1.0.0)."},
		{VersionImpact{Bump: "major", Before: "0.1.0", After: "1.0.0"}, "**major** bump (breaking change). The next release becomes **1.0.0**, up from 0.1.0 without this PR (first release)."},
	}
	for _, c := range cases {
		var b strings.Builder
		writeImpact(&b, &c.im)
		if !strings.Contains(b.String(), c.want) {
			t.Errorf("writeImpact(%+v) = %q, want it to contain %q", c.im, b.String(), c.want)
		}
	}
}

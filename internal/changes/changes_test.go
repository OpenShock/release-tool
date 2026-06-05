package changes

import (
	"os"
	"path/filepath"
	"testing"
)

// writeChange creates a .changes/<name> file in a temp dir.
func writeChange(t *testing.T, dir, name, content string) {
	t.Helper()
	changesDir := filepath.Join(dir, Dir)
	if err := os.MkdirAll(changesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changesDir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRead_Empty(t *testing.T) {
	dir := t.TempDir()
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ch) != 0 {
		t.Errorf("expected 0 changes, got %d", len(ch))
	}
}

func TestRead_Valid(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "my-feature.md", `---
kind: added
---
Add new endpoint

## Release Note
Short release note for consumers.

## Notices
- warning: requires firmware update
`)

	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ch) != 1 {
		t.Fatalf("expected 1 change, got %d", len(ch))
	}
	c := ch[0]
	if c.Kind != "added" {
		t.Errorf("Kind: got %q", c.Kind)
	}
	if c.Bump != "minor" {
		t.Errorf("Bump: got %q (added should derive minor)", c.Bump)
	}
	if c.Title != "Add new endpoint" {
		t.Errorf("Title: got %q", c.Title)
	}
	if c.ReleaseNote != "Short release note for consumers." {
		t.Errorf("ReleaseNote: got %q", c.ReleaseNote)
	}
	if len(c.Notices) != 1 || c.Notices[0].Level != "warning" || c.Notices[0].Message != "requires firmware update" {
		t.Errorf("Notices: got %+v", c.Notices)
	}
	if c.Breaking {
		t.Error("Breaking should be false when not set")
	}
	if c.Mandatory {
		t.Error("Mandatory should be false when not set")
	}
	if c.Filename != "my-feature.md" {
		t.Errorf("Filename: got %q", c.Filename)
	}
}

func TestRead_BreakingOverride(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "compat.md", `---
kind: removed
breaking: true
---
Remove deprecated field
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ch[0].Breaking {
		t.Error("expected Breaking=true")
	}
	if ch[0].Bump != "major" {
		t.Errorf("expected Bump=major for breaking, got %q", ch[0].Bump)
	}
}

func TestRead_MandatoryField(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "must-visit.md", `---
kind: changed
mandatory: true
---
Config format changed
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ch[0].Mandatory {
		t.Error("expected Mandatory=true")
	}
}

func TestRead_BumpDerivation(t *testing.T) {
	cases := []struct {
		kind     string
		breaking bool
		wantBump string
	}{
		{"fixed", false, "patch"},
		{"security", false, "patch"},
		{"added", false, "minor"},
		{"changed", false, "minor"},
		{"deprecated", false, "minor"},
		{"removed", false, "minor"},
		{"fixed", true, "major"},
		{"added", true, "major"},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		breaking := ""
		if tc.breaking {
			breaking = "\nbreaking: true"
		}
		writeChange(t, dir, "x.md", "---\nkind: "+tc.kind+breaking+"\n---\nTitle\n")
		ch, err := Read(dir)
		if err != nil {
			t.Fatalf("%s breaking=%v: unexpected error: %v", tc.kind, tc.breaking, err)
		}
		if ch[0].Bump != tc.wantBump {
			t.Errorf("%s breaking=%v: got Bump=%q, want %q", tc.kind, tc.breaking, ch[0].Bump, tc.wantBump)
		}
	}
}

func TestRead_SkipsReadmeAndHeadline(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "README.md", "# readme")
	writeChange(t, dir, "_headline.md", "Release headline")
	writeChange(t, dir, "real.md", `---
kind: fixed
---
Fix typo
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ch) != 1 {
		t.Errorf("expected 1 change (README and _headline skipped), got %d", len(ch))
	}
}

func TestRead_InvalidKind(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "bad.md", `---
kind: hotfix
---
Something
`)
	_, err := Read(dir)
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestRead_MissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "nofm.md", "just plain text\n")
	_, err := Read(dir)
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestRead_MissingTitle(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "notitle.md", `---
kind: fixed
---
`)
	_, err := Read(dir)
	if err == nil {
		t.Error("expected error for missing title")
	}
}

func TestRead_MultipleChangesOrdered(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "b-change.md", `---
kind: fixed
---
B change
`)
	writeChange(t, dir, "a-change.md", `---
kind: added
---
A change
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ch) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(ch))
	}
	if ch[0].Filename != "a-change.md" {
		t.Errorf("expected a-change.md first, got %q", ch[0].Filename)
	}
}

func TestReadHeadline(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, HeadlineFile, "  This is the headline.  \n")
	got := ReadHeadline(dir)
	if got != "This is the headline." {
		t.Errorf("got %q", got)
	}
}

func TestReadHeadline_Missing(t *testing.T) {
	dir := t.TempDir()
	got := ReadHeadline(dir)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestChange_Slug(t *testing.T) {
	c := &Change{Filename: "my-feature.md"}
	if c.Slug() != "my-feature" {
		t.Errorf("got %q", c.Slug())
	}
}

func TestRead_PRVerbatim(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "pinned.md", `---
kind: fixed
pr: 123
---
Fix with pinned PR
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch[0].PR == nil || *ch[0].PR != 123 {
		t.Errorf("expected PR=123, got %v", ch[0].PR)
	}
	if ch[0].PRExplicitNone {
		t.Error("PRExplicitNone should be false when a number is given")
	}
}

func TestRead_PRExplicitNone(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "nopr.md", `---
kind: fixed
pr: null
---
Fix without a PR
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch[0].PR != nil {
		t.Errorf("expected PR=nil, got %v", *ch[0].PR)
	}
	if !ch[0].PRExplicitNone {
		t.Error("expected PRExplicitNone=true for `pr: null`")
	}
}

func TestRead_PRAbsent(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "auto.md", `---
kind: fixed
---
Fix with derived PR
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch[0].PR != nil || ch[0].PRExplicitNone {
		t.Errorf("absent pr: should leave PR=nil and PRExplicitNone=false, got PR=%v none=%v", ch[0].PR, ch[0].PRExplicitNone)
	}
}

func TestRead_PRInvalid(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "badpr.md", `---
kind: fixed
pr: not-a-number
---
Fix
`)
	if _, err := Read(dir); err == nil {
		t.Error("expected error for non-integer pr")
	}
}

func TestRead_InvalidNoticeLevel(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "notice.md", `---
kind: fixed
---
Fix

## Notices
- warming: typo in the level
`)
	if _, err := Read(dir); err == nil {
		t.Error("expected error for invalid notice level")
	}
}

func TestRead_MalformedNotice(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "notice.md", `---
kind: fixed
---
Fix

## Notices
- this line has no colon separator
`)
	if _, err := Read(dir); err == nil {
		t.Error("expected error for malformed notice line")
	}
}

func TestReadSubset_SkipsMissing(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "present.md", `---
kind: fixed
---
Present change
`)
	ch, err := ReadSubset(dir, []string{"present.md", "absent.md"})
	if err != nil {
		t.Fatalf("missing file should be skipped, got: %v", err)
	}
	if len(ch) != 1 || ch[0].Filename != "present.md" {
		t.Errorf("expected only present.md, got %+v", ch)
	}
}

func TestReadSubset_BasenameGuard(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "real.md", `---
kind: fixed
---
Real change
`)
	ch, err := ReadSubset(dir, []string{"../../real.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ch) != 1 || ch[0].Filename != "real.md" {
		t.Errorf("expected basename-resolved real.md, got %+v", ch)
	}
}

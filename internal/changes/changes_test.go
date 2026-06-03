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
	// No .changes directory at all.
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
type: minor
categories: [api, firmware]
---
Add new endpoint

Extended description here.

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
	if c.Bump != "minor" {
		t.Errorf("Bump: got %q", c.Bump)
	}
	if c.Title != "Add new endpoint" {
		t.Errorf("Title: got %q", c.Title)
	}
	if c.Body != "Extended description here." {
		t.Errorf("Body: got %q", c.Body)
	}
	if c.ReleaseNote != "Short release note for consumers." {
		t.Errorf("ReleaseNote: got %q", c.ReleaseNote)
	}
	if len(c.Notices) != 1 || c.Notices[0].Level != "warning" || c.Notices[0].Message != "requires firmware update" {
		t.Errorf("Notices: got %+v", c.Notices)
	}
	if len(c.Categories) != 2 || c.Categories[0] != "api" || c.Categories[1] != "firmware" {
		t.Errorf("Categories: got %v", c.Categories)
	}
	if c.Breaking {
		t.Error("Breaking should be false for minor without explicit breaking:true")
	}
	if c.Filename != "my-feature.md" {
		t.Errorf("Filename: got %q", c.Filename)
	}
}

func TestRead_BreakingOverride(t *testing.T) {
	dir := t.TempDir()
	// minor type but explicitly marked breaking
	writeChange(t, dir, "compat.md", `---
type: minor
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
}

func TestRead_MajorDefaultsBreaking(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "big.md", `---
type: major
---
Big change
`)
	ch, err := Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ch[0].Breaking {
		t.Error("expected Breaking=true for major type")
	}
}

func TestRead_SkipsReadmeAndHeadline(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "README.md", "# readme")
	writeChange(t, dir, "_headline.md", "Release headline")
	writeChange(t, dir, "real.md", `---
type: patch
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

func TestRead_InvalidType(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "bad.md", `---
type: hotfix
---
Something
`)
	_, err := Read(dir)
	if err == nil {
		t.Error("expected error for unknown type")
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
type: patch
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
type: patch
---
B change
`)
	writeChange(t, dir, "a-change.md", `---
type: minor
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
	// Read sorts by filename alphabetically
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
type: patch
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
type: patch
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
type: patch
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
type: patch
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
type: patch
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
type: patch
---
Fix

## Notices
- this line has no colon separator
`)
	if _, err := Read(dir); err == nil {
		t.Error("expected error for malformed notice line")
	}
}

func TestRead_CategoryAllowlist(t *testing.T) {
	dir := t.TempDir()
	changesDir := filepath.Join(dir, Dir)
	if err := os.MkdirAll(changesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changesDir, ConfigFile),
		[]byte(`{"categories": ["api", "firmware"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	writeChange(t, dir, "ok.md", `---
type: minor
categories: [api]
---
Allowed category
`)
	if _, err := Read(dir); err != nil {
		t.Fatalf("allowed category should pass, got: %v", err)
	}

	writeChange(t, dir, "bad.md", `---
type: minor
categories: [typo]
---
Unknown category
`)
	if _, err := Read(dir); err == nil {
		t.Error("expected error for category outside the allowlist")
	}
}

func TestRead_CategoryNoAllowlist(t *testing.T) {
	dir := t.TempDir()
	// No config.json: any category is accepted (pre-existing behavior).
	writeChange(t, dir, "free.md", `---
type: minor
categories: [anything-goes]
---
Freeform category
`)
	if _, err := Read(dir); err != nil {
		t.Fatalf("category should be accepted without an allowlist, got: %v", err)
	}
}

package changes

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readOne(t *testing.T, content string) (*Change, error) {
	t.Helper()
	dir := t.TempDir()
	writeChange(t, dir, "c.md", content)
	ch, err := Read(dir)
	if err != nil {
		return nil, err
	}
	if len(ch) != 1 {
		t.Fatalf("expected 1 change, got %d", len(ch))
	}
	return ch[0], nil
}

func TestRead_NoticeLevelsAreAlertTypes(t *testing.T) {
	c, err := readOne(t, "---\nkind: fixed\n---\nFix it\n\n## Notices\n- caution: a\n- warning: b\n- IMPORTANT: c\n- note: d\n")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range c.Notices {
		got = append(got, n.Level)
	}
	if want := []string{"caution", "warning", "important", "note"}; !reflect.DeepEqual(got, want) {
		t.Errorf("levels = %v, want %v", got, want)
	}
	for _, old := range []string{"info", "error"} {
		if _, err := readOne(t, "---\nkind: fixed\n---\nFix it\n\n## Notices\n- "+old+": x\n"); err == nil {
			t.Errorf("legacy level %q should be rejected", old)
		}
	}
}

func TestRead_UnknownFrontmatterKeyRejected(t *testing.T) {
	_, err := readOne(t, "---\nkind: fixed\nbreakng: true\n---\nFix it\n")
	if err == nil || !strings.Contains(err.Error(), "breakng") {
		t.Errorf("expected an error naming the misspelt key, got %v", err)
	}
}

func TestRead_ExtraFields(t *testing.T) {
	c, err := readOne(t, `---
kind: security
upgrade:
  min_version: 1.4.0
  usb_only: true
  config_reset: true
  no_downgrade: true
advisories: [CVE-2026-12345, GHSA-jfh8-c2jp-5v3q]
authors: ["@someone", other-person]
yanks: [1.5.1]
---
Reject oversize messages
`)
	if err != nil {
		t.Fatal(err)
	}
	want := Upgrade{MinVersion: "1.4.0", USBOnly: true, ConfigReset: true, NoDowngrade: true}
	if c.Upgrade != want {
		t.Errorf("Upgrade = %+v, want %+v", c.Upgrade, want)
	}
	if !reflect.DeepEqual(c.Advisories, []string{"CVE-2026-12345", "GHSA-jfh8-c2jp-5v3q"}) {
		t.Errorf("Advisories = %v", c.Advisories)
	}
	if !reflect.DeepEqual(c.Authors, []string{"someone", "other-person"}) {
		t.Errorf("Authors = %v (a leading @ must be stripped)", c.Authors)
	}
	if !reflect.DeepEqual(c.Yanks, []string{"1.5.1"}) {
		t.Errorf("Yanks = %v", c.Yanks)
	}
}

func TestRead_ExtraFieldValidation(t *testing.T) {
	cases := map[string]string{
		"advisory needs security kind": "---\nkind: fixed\nadvisories: [CVE-2026-1234]\n---\nT\n",
		"advisory format":              "---\nkind: security\nadvisories: [CVE-1]\n---\nT\n",
		"min version format":           "---\nkind: fixed\nupgrade:\n  min_version: latest\n---\nT\n",
		"unknown upgrade key":          "---\nkind: fixed\nupgrade:\n  usb: true\n---\nT\n",
		"author login":                 "---\nkind: fixed\nauthors: [\"not a login\"]\n---\nT\n",
		"yank version":                 "---\nkind: fixed\nyanks: [v1]\n---\nT\n",
	}
	for name, content := range cases {
		if _, err := readOne(t, content); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestRead_ReservedUnderscoreFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "real.md", "---\nkind: fixed\n---\nFix it\n")
	writeChange(t, dir, "_known-issues.md", "- something\n")
	writeChange(t, dir, "_anything.md", "not a change file")
	ch, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 1 || ch[0].Filename != "real.md" {
		t.Errorf("only real.md should be read, got %+v", ch)
	}
}

func TestRead_ArchiveDirIgnored(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "real.md", "---\nkind: fixed\n---\nFix it\n")
	archived := filepath.Join(dir, Dir, ArchiveDir, "1.0.0")
	if err := os.MkdirAll(archived, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archived, "old.md"), []byte("---\nkind: fixed\n---\nOld\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ch, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 1 {
		t.Errorf("archived change files must not be read as pending, got %d", len(ch))
	}
}

func TestReadKnownIssues(t *testing.T) {
	dir := t.TempDir()
	if issues, err := ReadKnownIssues(dir); err != nil || issues != nil {
		t.Errorf("missing file: got %v, %v", issues, err)
	}
	writeChange(t, dir, KnownIssuesFile, "# Known issues\n\n- Pairing fails on USB power.\n  Use a wall adapter.\n- Second issue\n")
	issues, err := ReadKnownIssues(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Pairing fails on USB power. Use a wall adapter.", "Second issue"}
	if !reflect.DeepEqual(issues, want) {
		t.Errorf("issues = %q, want %q", issues, want)
	}
	writeChange(t, dir, KnownIssuesFile, "not a list item\n")
	if _, err := ReadKnownIssues(dir); err == nil {
		t.Error("a line that is not a list item should be an error")
	}
}

func TestInvalidFilesError_Findings(t *testing.T) {
	dir := t.TempDir()
	writeChange(t, dir, "bad.md", "---\nkind: nope\n---\nT\n")
	_, err := Read(dir)
	ife, ok := err.(*InvalidFilesError)
	if !ok {
		t.Fatalf("expected *InvalidFilesError, got %T", err)
	}
	if len(ife.Findings) != 1 || ife.Findings[0].File != "bad.md" || strings.HasPrefix(ife.Findings[0].Message, "bad.md") {
		t.Errorf("finding should carry the file apart from the message: %+v", ife.Findings)
	}
	if !strings.Contains(err.Error(), "bad.md: kind must be") {
		t.Errorf("Error() should keep the file-prefixed format: %v", err)
	}
}

func TestLint(t *testing.T) {
	ch := []*Change{
		{Filename: "a.md", Kind: "removed", Breaking: true, Title: "Remove X"},
		{Filename: "b.md", Kind: "removed", Breaking: true, Title: "Remove Y", ReleaseNote: "Y is gone; use Z."},
		{Filename: "c.md", Kind: "fixed", Title: "Fixed a crash", ReleaseNote: "x"},
		{Filename: "d.md", Kind: "fixed", Title: "Embed the font", ReleaseNote: "x"},
		{Filename: "e.md", Kind: "safety", Title: "Refuse X"},
		{Filename: "f.md", Kind: "added", Title: "Speed up boot"},
	}
	got := map[string]Severity{}
	for _, f := range Lint(ch) {
		got[f.File] = f.Severity
	}
	want := map[string]Severity{"a.md": SeverityError, "c.md": SeverityWarning, "e.md": SeverityWarning}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lint findings = %v, want %v", got, want)
	}
	if !HasErrors(Lint(ch)) {
		t.Error("HasErrors should be true")
	}
}

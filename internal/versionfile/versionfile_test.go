package versionfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetJSONString_KeepsFormatting(t *testing.T) {
	in := "{\n\t\"name\": \"frontend\",\n\t\"private\": true,\n\t\"version\" :  \"1.5.2\",\n\t\"nested\": {\"version\": \"x\"}\n}\n"
	want := "{\n\t\"name\": \"frontend\",\n\t\"private\": true,\n\t\"version\" :  \"1.6.0-rc.3\",\n\t\"nested\": {\"version\": \"x\"}\n}\n"
	got, err := SetJSONString([]byte(in), "version", "1.6.0-rc.3")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetJSONString_SkipsNestedKeys(t *testing.T) {
	in := `{"engines": {"version": "x"}, "version": "1.0.0"}`
	got, err := SetJSONString([]byte(in), "version", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"engines": {"version": "x"}, "version": "2.0.0"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestSetJSONString_Errors(t *testing.T) {
	cases := map[string]string{
		"missing key": `{"name": "x"}`,
		"not string":  `{"version": 3}`,
		"not object":  `["version"]`,
		"invalid":     `{"version": `,
	}
	for name, in := range cases {
		if _, err := SetJSONString([]byte(in), "version", "1.0.0"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestUpdate(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("frontend/package.json", `{"version": "1.0.0"}`)
	write("current.json", `{"version": "1.1.0"}`)

	changed, err := Update(root, []File{
		{Path: "frontend/package.json", JSONKey: "version"},
		{Path: "current.json", JSONKey: "version"},
	}, "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "frontend/package.json" {
		t.Errorf("changed = %v, want only frontend/package.json", changed)
	}
	data, _ := os.ReadFile(filepath.Join(root, "frontend", "package.json"))
	if string(data) != `{"version": "1.1.0"}` {
		t.Errorf("package.json = %s", data)
	}
}

func TestUpdate_BadEntryWritesNothing(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "package.json")
	if err := os.WriteFile(p, []byte(`{"version": "1.0.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []File{
		{Path: "missing.json", JSONKey: "version"},
		{Path: "../outside.json", JSONKey: "version"},
		{Path: "package.json"},
	} {
		_, err := Update(root, []File{{Path: "package.json", JSONKey: "version"}, bad}, "2.0.0")
		if err == nil {
			t.Errorf("%+v: expected an error", bad)
		}
		if data, _ := os.ReadFile(p); !strings.Contains(string(data), "1.0.0") {
			t.Fatalf("%+v: package.json was written despite the error: %s", bad, data)
		}
	}
}

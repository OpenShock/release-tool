package versionfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetJSONString_EdgeCases covers formats real version files come in and
// documents that would be misread or committed broken.
func TestSetJSONString_EdgeCases(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"crlf", "{\r\n\t\"name\": \"x\",\r\n\t\"version\": \"1.5.2\"\r\n}\r\n", "{\r\n\t\"name\": \"x\",\r\n\t\"version\": \"9.9.9-rc.1\"\r\n}\r\n", false},
		{"minified", `{"a":1,"version":"1.0.0","b":[1,2]}`, `{"a":1,"version":"9.9.9-rc.1","b":[1,2]}`, false},
		{"spaces around colon", "{ \"version\"   :\n   \"1.0.0\" ,\"x\":1}", "{ \"version\"   :\n   \"9.9.9-rc.1\" ,\"x\":1}", false},
		{"escaped key", `{"ver\u0073ion": "1.0.0"}`, `{"ver\u0073ion": "9.9.9-rc.1"}`, false},
		{"escaped value", `{"version": "1.0.0\u002b1"}`, `{"version": "9.9.9-rc.1"}`, false},
		{"unicode elsewhere", "{\"name\": \"\u00e6\u00f8\u00e5 \U0001F680\", \"version\": \"1.0.0\"}", "{\"name\": \"\u00e6\u00f8\u00e5 \U0001F680\", \"version\": \"9.9.9-rc.1\"}", false},
		{"version key in array before", `{"files": [{"version": "x"}], "version": "1.0.0"}`, `{"files": [{"version": "x"}], "version": "9.9.9-rc.1"}`, false},
		{"version as string value before", `{"label": "version", "version": "1.0.0"}`, `{"label": "version", "version": "9.9.9-rc.1"}`, false},
		{"empty value", `{"version": ""}`, `{"version": "9.9.9-rc.1"}`, false},
		{"utf8 bom", "\ufeff{\"version\": \"1.0.0\"}", "\ufeff{\"version\": \"9.9.9-rc.1\"}", false},
		{"bom + crlf", "\ufeff{\r\n\t\"version\": \"1\"\r\n}\r\n", "\ufeff{\r\n\t\"version\": \"9.9.9-rc.1\"\r\n}\r\n", false},
		{"double bom", "\ufeff\ufeff{\"version\": \"1\"}", "", true},
		{"duplicate keys", `{"version": "1.0.0", "version": "2.0.0"}`, "", true},
		{"duplicate of other key ok", `{"a": 1, "a": 2, "version": "1"}`, `{"a": 1, "a": 2, "version": "9.9.9-rc.1"}`, false},
		{"top-level array", `[{"version": "1"}]`, "", true},
		{"top-level string", `"version"`, "", true},
		{"missing key", `{"name": "x"}`, "", true},
		{"null value", `{"version": null}`, "", true},
		{"number value", `{"version": 1}`, "", true},
		{"empty file", ``, "", true},
		{"whitespace only", "  \n", "", true},
		{"jsonc comment", "{\n // c\n \"version\": \"1.0.0\"}", "", true},
		{"trailing comma", `{"version": "1.0.0",}`, "", true},
		{"trailing garbage", `{"version": "1.0.0"} garbage`, "", true},
		{"broken after key", `{"version": "1.0.0", oops}`, "", true},
		{"two objects", `{"version": "1"}{"version": "2"}`, "", true},
	}
	for _, c := range cases {
		got, err := SetJSONString([]byte(c.in), "version", "9.9.9-rc.1")
		switch {
		case c.wantErr && err == nil:
			t.Errorf("%s: expected error, got %q", c.name, got)
		case !c.wantErr && err != nil:
			t.Errorf("%s: unexpected error: %v", c.name, err)
		case !c.wantErr && string(got) != c.want:
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func fixtureRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestUpdate_EdgeCases(t *testing.T) {
	t.Run("two keys in one file", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"p.json": `{"version": "1", "appVersion": "1"}`})
		changed, err := Update(root, []File{{"p.json", "version"}, {"p.json", "appVersion"}}, "2")
		data, _ := os.ReadFile(filepath.Join(root, "p.json"))
		if err != nil || string(data) != `{"version": "2", "appVersion": "2"}` || len(changed) != 1 {
			t.Errorf("got %s err=%v changed=%v", data, err, changed)
		}
	})
	t.Run("same file spelled two ways", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"a/p.json": `{"version": "1"}`})
		changed, err := Update(root, []File{{"a/p.json", "version"}, {"./a/p.json", "version"}}, "2")
		if err != nil || len(changed) != 1 || changed[0] != "a/p.json" {
			t.Errorf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("already current", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"p.json": `{"version": "2"}`})
		changed, err := Update(root, []File{{"p.json", "version"}}, "2")
		if err != nil || changed != nil {
			t.Errorf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("second key missing writes nothing", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"p.json": `{"version": "1"}`})
		_, err := Update(root, []File{{"p.json", "version"}, {"p.json", "appVersion"}}, "2")
		data, _ := os.ReadFile(filepath.Join(root, "p.json"))
		if err == nil || string(data) != `{"version": "1"}` {
			t.Errorf("err=%v data=%s", err, data)
		}
	})
	t.Run("several files", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"a/package.json": `{"version": "1"}`, "b/manifest.json": `{"version": "1"}`})
		changed, err := Update(root, []File{{"a/package.json", "version"}, {"b/manifest.json", "version"}}, "2")
		if err != nil || len(changed) != 2 {
			t.Errorf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("no entries", func(t *testing.T) {
		changed, err := Update(t.TempDir(), nil, "2")
		if err != nil || changed != nil {
			t.Errorf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("bad paths rejected", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"dir/x.json": `{"version": "1"}`})
		for _, p := range []string{"../x.json", "/etc/x.json", "C:/x.json", "dir/../../x.json", "", "dir", "missing.json"} {
			if _, err := Update(root, []File{{p, "version"}}, "2"); err == nil {
				t.Errorf("path %q accepted", p)
			}
		}
	})
	t.Run("one bad among good writes nothing", func(t *testing.T) {
		root := fixtureRoot(t, map[string]string{"a.json": `{"version": "1"}`, "b.json": `{"v": "1"}`})
		_, err := Update(root, []File{{"a.json", "version"}, {"b.json", "version"}}, "2")
		data, _ := os.ReadFile(filepath.Join(root, "a.json"))
		if err == nil || !strings.Contains(string(data), `"1"`) {
			t.Errorf("err=%v a.json=%s", err, data)
		}
	})
}

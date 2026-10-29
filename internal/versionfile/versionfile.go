// Package versionfile rewrites the version recorded in project files (such as
// package.json) so a release commit carries the version it is tagged with.
package versionfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// File is one entry of version_files in .changes/config.json.
type File struct {
	// Path is relative to the repository root.
	Path string `json:"path"`
	// JSONKey is the top-level key of a JSON object whose string value is the
	// version, e.g. "version" in package.json.
	JSONKey string `json:"json_key"`
}

// Validate reports a config entry that cannot be applied.
func (f File) Validate() error {
	if f.Path == "" {
		return errors.New("version_files: entry without a path")
	}
	if !filepath.IsLocal(filepath.FromSlash(f.Path)) {
		return fmt.Errorf("version_files: %s must be a relative path inside the repository", f.Path)
	}
	if f.JSONKey == "" {
		return fmt.Errorf("version_files: %s has no json_key", f.Path)
	}
	return nil
}

// Update writes version into each file and returns the paths it changed, each
// once. Files that already hold version are left alone, and several entries
// for one file (e.g. two keys) all apply. Every entry is validated and
// rewritten in memory before any file is written, so a bad entry changes nothing.
func Update(root string, files []File, version string) ([]string, error) {
	type pending struct {
		orig, data []byte
		mode       os.FileMode
	}
	byPath := map[string]*pending{}
	var order []string
	for _, f := range files {
		if err := f.Validate(); err != nil {
			return nil, err
		}
		path := filepath.ToSlash(filepath.Clean(filepath.FromSlash(f.Path)))
		p, ok := byPath[path]
		if !ok {
			full := filepath.Join(root, filepath.FromSlash(path))
			info, err := os.Stat(full)
			if err != nil {
				return nil, fmt.Errorf("version_files: %w", err)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return nil, fmt.Errorf("version_files: %w", err)
			}
			p = &pending{orig: data, data: data, mode: info.Mode().Perm()}
			byPath[path] = p
			order = append(order, path)
		}
		out, err := SetJSONString(p.data, f.JSONKey, version)
		if err != nil {
			return nil, fmt.Errorf("version_files: %s: %w", f.Path, err)
		}
		p.data = out
	}

	var changed []string
	for _, path := range order {
		p := byPath[path]
		if bytes.Equal(p.data, p.orig) {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), p.data, p.mode); err != nil {
			return changed, fmt.Errorf("version_files: %w", err)
		}
		changed = append(changed, path)
	}
	return changed, nil
}

// utf8BOM is kept in place when a file starts with it; json rejects it.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// SetJSONString replaces the string value of a top-level key in a JSON object,
// splicing only the value's bytes so indentation, key order and the rest of
// the file are kept exactly as they were. The whole document must be valid
// JSON, and the key must appear exactly once: with duplicates, JSON.parse
// would read a different value than the one replaced.
func SetJSONString(data []byte, key, value string) ([]byte, error) {
	body := bytes.TrimPrefix(data, utf8BOM)
	skip := len(data) - len(body)
	if !json.Valid(body) {
		var v any
		err := json.Unmarshal(body, &v)
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	start, end := -1, -1
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		k, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		if k != key {
			continue
		}
		if start >= 0 {
			return nil, fmt.Errorf("top-level key %q appears more than once", key)
		}
		if len(raw) == 0 || raw[0] != '"' {
			return nil, fmt.Errorf("%q is not a string", key)
		}
		end = skip + int(dec.InputOffset())
		start = end - len(raw)
	}
	if start < 0 {
		return nil, fmt.Errorf("no top-level key %q", key)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return slices.Concat(data[:start], encoded, data[end:]), nil
}

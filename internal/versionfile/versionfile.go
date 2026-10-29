// Package versionfile rewrites the version recorded in project files (such as
// package.json) so a release commit carries the version it is tagged with.
package versionfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// Update writes version into each file and returns the paths it changed.
// Files that already hold version are left alone. Every entry is validated and
// rewritten in memory before any file is written, so a bad entry changes nothing.
func Update(root string, files []File, version string) ([]string, error) {
	type pending struct {
		path string
		data []byte
		mode os.FileMode
	}
	var writes []pending
	for _, f := range files {
		if err := f.Validate(); err != nil {
			return nil, err
		}
		full := filepath.Join(root, filepath.FromSlash(f.Path))
		info, err := os.Stat(full)
		if err != nil {
			return nil, fmt.Errorf("version_files: %w", err)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("version_files: %w", err)
		}
		out, err := SetJSONString(data, f.JSONKey, version)
		if err != nil {
			return nil, fmt.Errorf("version_files: %s: %w", f.Path, err)
		}
		if bytes.Equal(out, data) {
			continue
		}
		writes = append(writes, pending{f.Path, out, info.Mode().Perm()})
	}

	var changed []string
	for _, w := range writes {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(w.path)), w.data, w.mode); err != nil {
			return changed, fmt.Errorf("version_files: %w", err)
		}
		changed = append(changed, w.path)
	}
	return changed, nil
}

// SetJSONString replaces the string value of a top-level key in a JSON object,
// splicing only the value's bytes so indentation, key order and the rest of
// the file are kept exactly as they were.
func SetJSONString(data []byte, key, value string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
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
		if len(raw) == 0 || raw[0] != '"' {
			return nil, fmt.Errorf("%q is not a string", key)
		}
		end := int(dec.InputOffset())
		start := end - len(raw)
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 0, len(data)-len(raw)+len(encoded))
		out = append(out, data[:start]...)
		out = append(out, encoded...)
		return append(out, data[end:]...), nil
	}
	if _, err := dec.Token(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return nil, fmt.Errorf("no top-level key %q", key)
}

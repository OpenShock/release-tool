package changes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Dir          = ".changes"
	HeadlineFile = "_headline.md"
	ConfigFile   = "config.json"
)

type ReleaseMode string

const (
	ReleaseModeNone       ReleaseMode = "none"
	ReleaseModeStable     ReleaseMode = "stable"
	ReleaseModePrerelease ReleaseMode = "prerelease"
)

// BranchConfig describes how a branch behaves for releases and PR checks.
// A branch present in the map requires a change file on PRs targeting it.
type BranchConfig struct {
	Release ReleaseMode `json:"release,omitempty"`
	Label   string      `json:"label,omitempty"`
	SHA     bool        `json:"sha,omitempty"`
}

type Config struct {
	TagPrefix string                  `json:"tag_prefix"`
	Branches  map[string]BranchConfig `json:"branches"`
}

func ReadConfig(root string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(root, Dir, ConfigFile))
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ConfigFile, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ConfigFile, err)
	}
	return &cfg, nil
}

type Notice struct {
	Level   string
	Message string
}

type Change struct {
	Kind        string
	Bump        string // derived: major | minor | patch
	Title       string
	ReleaseNote string
	Notices     []Notice
	Filename    string
	Breaking    bool
	Mandatory   bool
	// PR, when non-nil, is the verbatim PR number set in frontmatter, used
	// as-is instead of deriving from git history. PRExplicitNone records an
	// explicit `pr: null`, which suppresses PR derivation entirely.
	PR             *int
	PRExplicitNone bool
}

func (c *Change) Slug() string {
	return strings.TrimSuffix(c.Filename, ".md")
}

var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n(.*)$`)

type rawFrontmatter struct {
	Kind      string `yaml:"kind"`
	Breaking  *bool  `yaml:"breaking"`
	Mandatory *bool  `yaml:"mandatory"`
	// PR is a raw node so we can distinguish absent (derive), explicit null
	// (suppress), and an integer (verbatim). A zero Kind means the key was
	// absent. (Must be a value, not a pointer: yaml.v3 only special-cases
	// decoding into yaml.Node by value.)
	PR yaml.Node `yaml:"pr"`
}

// ValidKinds is the canonical, ordered list of change kinds. It is the single
// source of truth: the parser, the `new` command, and check messages all derive
// from it so the accepted set and the human-readable list cannot drift apart.
var ValidKinds = []string{"added", "changed", "deprecated", "removed", "fixed", "security", "safety", "chore"}

var validKinds = func() map[string]bool {
	m := make(map[string]bool, len(ValidKinds))
	for _, k := range ValidKinds {
		m[k] = true
	}
	return m
}()

// IsValidKind reports whether k is a recognised change kind.
func IsValidKind(k string) bool { return validKinds[k] }

// KindList renders the valid kinds as a `a|b|c` string for error and help text.
func KindList() string { return strings.Join(ValidKinds, "|") }

var knownSections = map[string]bool{
	"release note": true,
	"notices":      true,
}

// splitSections partitions the body into the changelog title region and the
// `## Release Note` / `## Notices` sections. Any `## ` heading that is not a
// known section, or a known section that appears twice, is an error rather than
// being silently swept into the title region or having its first block dropped.
func splitSections(body string) (changelog, releaseNote, notices string, err error) {
	sections := map[string][]string{"_changelog": {}}
	current := "_changelog"
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			key := strings.ToLower(strings.TrimSpace(trimmed[3:]))
			if !knownSections[key] {
				return "", "", "", fmt.Errorf("unknown section %q (expected `## Release Note` or `## Notices`)", trimmed)
			}
			if _, seen := sections[key]; seen {
				return "", "", "", fmt.Errorf("duplicate section %q", trimmed)
			}
			sections[key] = nil
			current = key
			continue
		}
		sections[current] = append(sections[current], line)
	}
	join := func(k string) string { return strings.TrimSpace(strings.Join(sections[k], "\n")) }
	return join("_changelog"), join("release note"), join("notices"), nil
}

var validNoticeLevels = map[string]bool{"info": true, "warning": true, "error": true}

func parseNotices(raw string) ([]Notice, error) {
	var (
		out  []Notice
		errs []string
	)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			errs = append(errs, fmt.Sprintf("malformed notice %q (must be `- level: message`)", line))
			continue
		}
		body := strings.TrimSpace(line[2:])
		idx := strings.Index(body, ":")
		if idx < 0 {
			errs = append(errs, fmt.Sprintf("malformed notice %q (missing `:` separator)", line))
			continue
		}
		level := strings.ToLower(strings.TrimSpace(body[:idx]))
		if !validNoticeLevels[level] {
			errs = append(errs, fmt.Sprintf("invalid notice level %q (must be info, warning, or error)", level))
			continue
		}
		out = append(out, Notice{
			Level:   level,
			Message: strings.TrimSpace(body[idx+1:]),
		})
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return out, nil
}

func DeriveBump(kind string, breaking bool) string {
	if breaking {
		return "major"
	}
	switch kind {
	case "fixed", "security", "safety", "chore":
		return "patch"
	}
	return "minor"
}

func parseFile(path string) (*Change, error) {
	filename := filepath.Base(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read: %w", filename, err)
	}

	m := frontmatterRe.FindSubmatch(content)
	if m == nil {
		return nil, fmt.Errorf("%s: missing YAML frontmatter", filename)
	}

	var fm rawFrontmatter
	if err := yaml.Unmarshal(m[1], &fm); err != nil {
		return nil, fmt.Errorf("%s: invalid YAML: %w", filename, err)
	}

	if !validKinds[fm.Kind] {
		return nil, fmt.Errorf("%s: kind must be %s, got %q", filename, KindList(), fm.Kind)
	}

	breaking := false
	if fm.Breaking != nil {
		breaking = *fm.Breaking
	}

	mandatory := false
	if fm.Mandatory != nil {
		mandatory = *fm.Mandatory
	}

	changelogRaw, releaseNote, noticesRaw, err := splitSections(strings.TrimSpace(string(m[2])))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if changelogRaw == "" {
		return nil, fmt.Errorf("%s: missing title line", filename)
	}

	title := strings.TrimSpace(strings.SplitN(changelogRaw, "\n", 2)[0])
	if title == "" {
		return nil, fmt.Errorf("%s: title line is empty", filename)
	}

	var bodyLines int
	for _, l := range strings.Split(changelogRaw, "\n") {
		if strings.TrimSpace(l) != "" {
			bodyLines++
		}
	}
	if bodyLines > 1 {
		return nil, fmt.Errorf("%s: only a single title line is allowed above sections, got %d lines", filename, bodyLines)
	}

	notices, err := parseNotices(noticesRaw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}

	var prNum *int
	prExplicitNone := false
	if fm.PR.Kind != 0 { // key present
		if fm.PR.Tag == "!!null" {
			prExplicitNone = true
		} else {
			n, convErr := strconv.Atoi(strings.TrimSpace(fm.PR.Value))
			if convErr != nil {
				return nil, fmt.Errorf("%s: pr must be a positive integer or null, got %q", filename, fm.PR.Value)
			}
			if n <= 0 {
				return nil, fmt.Errorf("%s: pr must be a positive integer, got %d", filename, n)
			}
			prNum = &n
		}
	}

	return &Change{
		Kind:           fm.Kind,
		Bump:           DeriveBump(fm.Kind, breaking),
		Title:          title,
		ReleaseNote:    releaseNote,
		Notices:        notices,
		Filename:       filename,
		Breaking:       breaking,
		Mandatory:      mandatory,
		PR:             prNum,
		PRExplicitNone: prExplicitNone,
	}, nil
}

// invalidFilesErr aggregates per-file parse/validation errors into one error.
func invalidFilesErr(errs []string) error {
	return fmt.Errorf("invalid change files:\n  - %s", strings.Join(errs, "\n  - "))
}

// parseAll parses each path into a Change, aggregating errors. When skipMissing
// is set, non-existent files are silently skipped instead of reported.
func parseAll(paths []string, skipMissing bool) ([]*Change, error) {
	var (
		out  []*Change
		errs []string
	)
	for _, p := range paths {
		ch, err := parseFile(p)
		if err != nil {
			if skipMissing && errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, ch)
	}
	if len(errs) > 0 {
		return nil, invalidFilesErr(errs)
	}
	return out, nil
}

func Read(root string) ([]*Change, error) {
	dir := filepath.Join(root, Dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", Dir, err)
	}

	var paths []string
	for _, e := range entries {
		name := e.Name()
		lower := strings.ToLower(name)
		if e.IsDir() || !strings.HasSuffix(lower, ".md") || lower == "readme.md" || lower == "_headline.md" {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)

	return parseAll(paths, false)
}

// ReadSubset reads and parses only the named files (basenames) from .changes/.
// Missing files are silently skipped. Names are reduced to their basename so a
// caller cannot escape .changes/ via path separators, and duplicate basenames
// (e.g. a file added, removed, then re-added across commits) are read once.
func ReadSubset(root string, filenames []string) ([]*Change, error) {
	bases := make([]string, 0, len(filenames))
	seen := make(map[string]bool, len(filenames))
	for _, name := range filenames {
		base := filepath.Base(name)
		if seen[base] {
			continue
		}
		seen[base] = true
		bases = append(bases, base)
	}
	sort.Strings(bases)

	paths := make([]string, 0, len(bases))
	for _, base := range bases {
		paths = append(paths, filepath.Join(root, Dir, base))
	}

	return parseAll(paths, true)
}

func ReadHeadline(root string) string {
	data, err := os.ReadFile(filepath.Join(root, Dir, HeadlineFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

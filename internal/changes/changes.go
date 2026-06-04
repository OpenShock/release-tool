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

type Config struct {
	TagPrefix string `json:"tag_prefix"`
	// Categories, when non-empty, is the allowlist of category names a change
	// file may declare. When empty, any category string is accepted.
	Categories []string `json:"categories"`
	// Branches maps a release branch name to its lane (stable|beta|develop).
	// It is the single source of truth for which branches are release branches,
	// used by the check command to recognise a pull request's base branch.
	Branches map[string]string `json:"branches"`
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
	Bump        string
	Title       string
	Body        string
	ReleaseNote string
	Notices     []Notice
	Filename    string
	Breaking    bool
	Categories  []string
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
	Type       string   `yaml:"type"`
	Breaking   *bool    `yaml:"breaking"`
	Categories []string `yaml:"categories"`
	// PR is a raw node so we can distinguish absent (derive), explicit null
	// (suppress), and an integer (verbatim). A zero Kind means the key was
	// absent. (Must be a value, not a pointer: yaml.v3 only special-cases
	// decoding into yaml.Node by value.)
	PR yaml.Node `yaml:"pr"`
}

var knownSections = map[string]bool{
	"## Release Note": true,
	"## Notices":      true,
}

func splitSections(body string) (changelog, releaseNote, notices string) {
	sections := map[string][]string{"_changelog": {}}
	current := "_changelog"
	for _, line := range strings.Split(body, "\n") {
		if knownSections[strings.TrimSpace(line)] {
			current = strings.ToLower(strings.TrimSpace(line)[3:])
			sections[current] = nil
			continue
		}
		sections[current] = append(sections[current], line)
	}
	join := func(k string) string { return strings.TrimSpace(strings.Join(sections[k], "\n")) }
	return join("_changelog"), join("release note"), join("notices")
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

	switch fm.Type {
	case "major", "minor", "patch":
	default:
		return nil, fmt.Errorf("%s: type must be major|minor|patch, got %q", filename, fm.Type)
	}

	breaking := fm.Type == "major"
	if fm.Breaking != nil {
		breaking = *fm.Breaking
	}

	changelogRaw, releaseNote, noticesRaw := splitSections(strings.TrimSpace(string(m[2])))
	if changelogRaw == "" {
		return nil, fmt.Errorf("%s: missing title line", filename)
	}

	title, body, _ := strings.Cut(changelogRaw, "\n")
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%s: title line is empty", filename)
	}

	categories := fm.Categories
	if categories == nil {
		categories = []string{}
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
				return nil, fmt.Errorf("%s: pr must be an integer or null, got %q", filename, fm.PR.Value)
			}
			prNum = &n
		}
	}

	return &Change{
		Bump:           fm.Type,
		Title:          title,
		Body:           strings.TrimSpace(body),
		ReleaseNote:    releaseNote,
		Notices:        notices,
		Filename:       filename,
		Breaking:       breaking,
		Categories:     categories,
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

	out, err := parseAll(paths, false)
	if err != nil {
		return nil, err
	}
	if err := validateCategories(root, out); err != nil {
		return nil, err
	}
	return out, nil
}

// validateCategories rejects change files declaring categories outside the
// allowlist in config.json. When the allowlist is empty, any category is
// accepted (the pre-existing behavior).
func validateCategories(root string, list []*Change) error {
	cfg, err := ReadConfig(root)
	if err != nil {
		return err
	}
	if len(cfg.Categories) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(cfg.Categories))
	for _, c := range cfg.Categories {
		allowed[c] = true
	}
	var errs []string
	for _, ch := range list {
		for _, cat := range ch.Categories {
			if !allowed[cat] {
				errs = append(errs, fmt.Sprintf("%s: unknown category %q (allowed: %s)",
					ch.Filename, cat, strings.Join(cfg.Categories, ", ")))
			}
		}
	}
	if len(errs) > 0 {
		return invalidFilesErr(errs)
	}
	return nil
}

// ReadSubset reads and parses only the named files (basenames) from .changes/.
// Missing files are silently skipped. Names are reduced to their basename so a
// caller cannot escape .changes/ via path separators.
func ReadSubset(root string, filenames []string) ([]*Change, error) {
	sorted := make([]string, len(filenames))
	copy(sorted, filenames)
	sort.Strings(sorted)

	paths := make([]string, 0, len(sorted))
	for _, name := range sorted {
		paths = append(paths, filepath.Join(root, Dir, filepath.Base(name)))
	}

	out, err := parseAll(paths, true)
	if err != nil {
		return nil, err
	}
	if err := validateCategories(root, out); err != nil {
		return nil, err
	}
	return out, nil
}

func ReadHeadline(root string) string {
	data, err := os.ReadFile(filepath.Join(root, Dir, HeadlineFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

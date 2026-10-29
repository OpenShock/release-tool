package changes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/OpenShock/release-tool/internal/versionfile"
	"gopkg.in/yaml.v3"
)

const (
	Dir          = ".changes"
	HeadlineFile = "_headline.md"
	// KnownIssuesFile lists open known issues. Unlike the headline it is not
	// consumed by a stable release: each issue stays until someone removes it.
	KnownIssuesFile = "_known-issues.md"
	ConfigFile      = "config.json"
	// ArchiveDir is where a stable release moves the change files it consumed,
	// as .changes/released/<version>/<file>, so past notes stay editable.
	ArchiveDir = "released"
)

// IsChangeFileName reports whether a file in .changes/ is a change file. Names
// starting with "_" are reserved for special files (headline, known issues).
func IsChangeFileName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") && lower != "readme.md" && !strings.HasPrefix(lower, "_")
}

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
	// Maintainers are logins always excluded from the contributors footer,
	// independent of the gh-derived collaborator set (which is unavailable when
	// the token lacks collaborator-read, e.g. the default Actions token).
	Maintainers []string `json:"maintainers,omitempty"`
	// PrereleaseNotice replaces the warning that opens a prerelease's GitHub
	// Release body. "{stable}" is replaced with the latest stable tag.
	PrereleaseNotice string `json:"prerelease_notice,omitempty"`
	// IgnorePaths are globs (`*`, `**`, `?`) for files that never need a change
	// file. A pull request that only touches these paths passes the check.
	IgnorePaths []string `json:"ignore_paths,omitempty"`
	// FoldAfter folds a release body section with more than this many entries
	// into a collapsed <details> block. 0 uses the default; negative disables.
	FoldAfter int `json:"fold_after,omitempty"`
	// VersionFiles are set to the released version in the release commit, by
	// stable releases and by prereleases that are not SHA builds.
	VersionFiles []versionfile.File `json:"version_files,omitempty"`
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

// Upgrade describes what a user has to know or do before installing a
// release. Every field is optional; a release aggregates them across changes.
type Upgrade struct {
	// MinVersion is the oldest version that can update to this one directly.
	MinVersion string `yaml:"min_version" json:"min_version,omitempty"`
	// USBOnly means the update cannot be installed over the air.
	USBOnly bool `yaml:"usb_only" json:"usb_only,omitempty"`
	// ConfigReset means the update discards the stored configuration.
	ConfigReset bool `yaml:"config_reset" json:"config_reset,omitempty"`
	// NoDowngrade means earlier versions cannot be installed afterwards.
	NoDowngrade bool `yaml:"no_downgrade" json:"no_downgrade,omitempty"`
}

// IsZero reports whether u sets nothing.
func (u Upgrade) IsZero() bool { return u == Upgrade{} }

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
	Upgrade        Upgrade
	// Advisories are CVE or GHSA identifiers fixed by a security change.
	Advisories []string
	// Authors are GitHub logins credited for the change, for when the commit
	// author is not the person to thank (squash merges, co-authored work).
	Authors []string
	// Yanks lists earlier versions this release withdraws.
	Yanks []string
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
	PR         yaml.Node `yaml:"pr"`
	Upgrade    Upgrade   `yaml:"upgrade"`
	Advisories []string  `yaml:"advisories"`
	Authors    []string  `yaml:"authors"`
	Yanks      []string  `yaml:"yanks"`
}

var (
	versionRe  = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	advisoryRe = regexp.MustCompile(`^(CVE-\d{4}-\d{4,}|GHSA(-[23456789cfghjmpqrvwx]{4}){3})$`)
	loginRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$`)
)

// validateExtras checks the optional frontmatter fields and normalises them in
// place (authors lose a leading "@").
func validateExtras(fm *rawFrontmatter) []string {
	var errs []string
	if v := fm.Upgrade.MinVersion; v != "" && !versionRe.MatchString(v) {
		errs = append(errs, fmt.Sprintf("upgrade.min_version must be a version like 1.2.3, got %q", v))
	}
	for _, a := range fm.Advisories {
		if !advisoryRe.MatchString(a) {
			errs = append(errs, fmt.Sprintf("advisory %q must be a CVE-YYYY-NNNN or GHSA-xxxx-xxxx-xxxx id", a))
		}
	}
	if len(fm.Advisories) > 0 && fm.Kind != "security" {
		errs = append(errs, fmt.Sprintf("advisories require kind: security, got %q", fm.Kind))
	}
	for i, a := range fm.Authors {
		login := strings.TrimPrefix(strings.TrimSpace(a), "@")
		if !loginRe.MatchString(login) {
			errs = append(errs, fmt.Sprintf("author %q is not a GitHub login", a))
		}
		fm.Authors[i] = login
	}
	for _, v := range fm.Yanks {
		if !versionRe.MatchString(v) {
			errs = append(errs, fmt.Sprintf("yanks entry must be a version like 1.2.3, got %q", v))
		}
	}
	return errs
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

// NoticeLevels are the accepted notice levels, most severe first. They map 1:1
// to GitHub alert types, which is how the release body renders them.
var NoticeLevels = []string{"caution", "warning", "important", "note"}

var validNoticeLevels = func() map[string]bool {
	m := make(map[string]bool, len(NoticeLevels))
	for _, l := range NoticeLevels {
		m[l] = true
	}
	return m
}()

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
			errs = append(errs, fmt.Sprintf("invalid notice level %q (must be %s)", level, strings.Join(NoticeLevels, ", ")))
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
	dec := yaml.NewDecoder(bytes.NewReader(m[1]))
	dec.KnownFields(true) // a misspelt key must not be silently ignored
	if err := dec.Decode(&fm); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: invalid YAML: %w", filename, err)
	}

	if !validKinds[fm.Kind] {
		return nil, fmt.Errorf("%s: kind must be %s, got %q", filename, KindList(), fm.Kind)
	}
	if errs := validateExtras(&fm); len(errs) > 0 {
		return nil, fmt.Errorf("%s: %s", filename, strings.Join(errs, "; "))
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
		Upgrade:        fm.Upgrade,
		Advisories:     fm.Advisories,
		Authors:        fm.Authors,
		Yanks:          fm.Yanks,
	}, nil
}

// InvalidFilesError aggregates per-file parse/validation errors. Findings keep
// the file apart from the message so CI can annotate the right file.
type InvalidFilesError struct {
	Findings []Finding
}

func (e *InvalidFilesError) Error() string {
	lines := make([]string, len(e.Findings))
	for i, f := range e.Findings {
		lines[i] = f.File + ": " + f.Message
	}
	return fmt.Sprintf("invalid change files:\n  - %s", strings.Join(lines, "\n  - "))
}

// parseAll parses each path into a Change, aggregating errors. When skipMissing
// is set, non-existent files are silently skipped instead of reported.
func parseAll(paths []string, skipMissing bool) ([]*Change, error) {
	var (
		out  []*Change
		errs []Finding
	)
	for _, p := range paths {
		ch, err := parseFile(p)
		if err != nil {
			if skipMissing && errors.Is(err, os.ErrNotExist) {
				continue
			}
			name := filepath.Base(p)
			errs = append(errs, Finding{name, SeverityError, strings.TrimPrefix(err.Error(), name+": ")})
			continue
		}
		out = append(out, ch)
	}
	if len(errs) > 0 {
		return nil, &InvalidFilesError{Findings: errs}
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
		if e.IsDir() || !IsChangeFileName(name) {
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

// ReadKnownIssues returns the items of .changes/_known-issues.md, one per
// `- ` list item. Indented lines continue the previous item. A missing file
// means no known issues.
func ReadKnownIssues(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, Dir, KnownIssuesFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", KnownIssuesFile, err)
	}
	var issues []string
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", strings.HasPrefix(trimmed, "#"):
		case strings.HasPrefix(trimmed, "- "):
			issues = append(issues, strings.TrimSpace(trimmed[2:]))
		case line != trimmed && len(issues) > 0:
			issues[len(issues)-1] += " " + trimmed
		default:
			return nil, fmt.Errorf("%s:%d: expected a `- ` list item, got %q", KnownIssuesFile, i+1, trimmed)
		}
	}
	return issues, nil
}

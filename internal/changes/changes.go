package changes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	Bump       string
	Title      string
	Body       string
	ReleaseNote string
	Notices    []Notice
	Filename   string
	Breaking   bool
	Categories []string
}

func (c *Change) Slug() string {
	return strings.TrimSuffix(c.Filename, ".md")
}

var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n(.*)$`)

type rawFrontmatter struct {
	Type       string   `yaml:"type"`
	Breaking   *bool    `yaml:"breaking"`
	Categories []string `yaml:"categories"`
}

var knownSections = map[string]bool{
	"## Release Note": true,
	"## Notices": true,
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

func parseNotices(raw string) []Notice {
	var out []Notice
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "- ") {
			continue
		}
		body := strings.TrimSpace(line[2:])
		idx := strings.Index(body, ":")
		if idx < 0 {
			continue
		}
		out = append(out, Notice{
			Level:   strings.ToLower(strings.TrimSpace(body[:idx])),
			Message: strings.TrimSpace(body[idx+1:]),
		})
	}
	return out
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

	return &Change{
		Bump:       fm.Type,
		Title:      title,
		Body:       strings.TrimSpace(body),
		ReleaseNote: releaseNote,
		Notices:    parseNotices(noticesRaw),
		Filename:   filename,
		Breaking:   breaking,
		Categories: categories,
	}, nil
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

	var (
		out  []*Change
		errs []string
	)
	for _, p := range paths {
		ch, err := parseFile(p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, ch)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid change files:\n  - %s", strings.Join(errs, "\n  - "))
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

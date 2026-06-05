package release

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
)

const schemaVersion = 1

type Repository struct {
	Platform string `json:"platform"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
}

type NoticeEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type ReleaseNoteEntry struct {
	Title       string   `json:"title"`
	Description []string `json:"description,omitempty"`
}

type ChangeEntry struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Breaking    bool              `json:"breaking"`
	Mandatory   bool              `json:"mandatory"`
	Title       string            `json:"title"`
	ReleaseNote *ReleaseNoteEntry `json:"release_note,omitempty"`
	PR          *int              `json:"pr,omitempty"`
	Notices     []NoticeEntry     `json:"notices"`
}

type ReleaseData struct {
	SchemaVersion   int           `json:"schema_version"`
	Repository      *Repository   `json:"repository,omitempty"`
	Version         string        `json:"version"`
	Tag             string        `json:"tag"`
	Prerelease      bool          `json:"prerelease"`
	PreviousVersion *string       `json:"previous_version,omitempty"`
	PreviousTag     string        `json:"previous_tag,omitempty"`
	ReleasedAt      string        `json:"released_at"`
	Commit          string        `json:"commit"`
	Mandatory       bool          `json:"mandatory"`
	Headline        string        `json:"headline,omitempty"`
	Changes         []ChangeEntry `json:"changes"`
	Contributors    []string      `json:"contributors"`
}

type BuildParams struct {
	Tag         string
	Previous    string
	PreviousTag string // literal previous tag (with prefix); ref for contributors compare
	Changes     []*changes.Change
	Headline    string
	Prerelease  bool
	Commit      string
	Version     string
	Root        string
	EnrichPR    bool
	GithubRepo string // e.g. "OpenShock/Firmware" from GITHUB_REPOSITORY
}

func parseRepository(githubRepo string) *Repository {
	parts := strings.SplitN(githubRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return &Repository{Platform: "github", Owner: parts[0], Repo: parts[1]}
}

func releaseNote(text string) *ReleaseNoteEntry {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	title, rest, _ := strings.Cut(text, "\n")
	title = strings.TrimSpace(title)
	rest = strings.TrimSpace(rest)
	entry := &ReleaseNoteEntry{Title: title}
	if rest != "" {
		var desc []string
		for _, line := range strings.Split(rest, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				desc = append(desc, trimmed)
			}
		}
		if len(desc) > 0 {
			entry.Description = desc
		}
	}
	return entry
}

func BuildData(p BuildParams) *ReleaseData {
	data := &ReleaseData{
		SchemaVersion: schemaVersion,
		Repository:    parseRepository(p.GithubRepo),
		Version:       p.Version,
		Tag:           p.Tag,
		Prerelease:    p.Prerelease,
		ReleasedAt:    time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		Commit:        p.Commit,
		Headline: strings.TrimSpace(p.Headline),
	}
	if p.Previous != "" {
		prev := p.Previous
		data.PreviousVersion = &prev
	}
	if p.PreviousTag != "" {
		data.PreviousTag = p.PreviousTag
	}

	for _, c := range p.Changes {
		if c.Mandatory {
			data.Mandatory = true
		}
		entry := ChangeEntry{
			ID:          c.Slug(),
			Kind:        c.Kind,
			Breaking:    c.Breaking,
			Mandatory:   c.Mandatory,
			Title:       c.Title,
			ReleaseNote: releaseNote(c.ReleaseNote),
			Notices:     make([]NoticeEntry, len(c.Notices)),
		}
		for i, n := range c.Notices {
			entry.Notices[i] = NoticeEntry{Level: n.Level, Message: n.Message}
		}
		switch {
		case c.PRExplicitNone:
			// explicit `pr: null` suppresses the PR link
		case c.PR != nil:
			pr := *c.PR
			entry.PR = &pr
		case p.EnrichPR:
			if n := git.DerivePRNumber(p.Root, c.Filename); n != 0 {
				entry.PR = &n
			}
		}
		data.Changes = append(data.Changes, entry)
	}

	data.Contributors = []string{}
	if p.EnrichPR && p.PreviousTag != "" {
		if c := git.ContributorsSince(p.Root, p.PreviousTag); c != nil {
			data.Contributors = c
		}
	}

	return data
}

func WriteJSON(path string, data *ReleaseData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling release.json: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", path)
	return nil
}

func WriteNotes(path string, data *ReleaseData, maintainers map[string]bool) error {
	content := RenderNotes(data, maintainers)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", path)
	return nil
}

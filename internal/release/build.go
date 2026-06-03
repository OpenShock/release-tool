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

type MDText struct {
	Format string `json:"format"`
	Text   string `json:"text"`
}

type NoticeEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type ChangeEntry struct {
	ID          string        `json:"id"`
	Type        string        `json:"type"`
	Breaking    bool          `json:"breaking"`
	Categories  []string      `json:"categories"`
	Title       MDText        `json:"title"`
	Body        *MDText       `json:"body,omitempty"`
	ReleaseNote *MDText       `json:"release_note,omitempty"`
	PR          *int          `json:"pr,omitempty"`
	Notices     []NoticeEntry `json:"notices"`
}

type ReleaseData struct {
	SchemaVersion   int           `json:"schema_version"`
	Version         string        `json:"version"`
	Tag             string        `json:"tag"`
	Prerelease      bool          `json:"prerelease"`
	PreviousVersion *string       `json:"previous_version"`
	ReleasedAt      string        `json:"released_at"`
	Commit          string        `json:"commit"`
	Headline        *MDText       `json:"headline"`
	Changes         []ChangeEntry `json:"changes"`
	Contributors    []string      `json:"contributors"`
}

type BuildParams struct {
	Tag         string
	Previous    string
	PreviousTag string // literal previous tag (with prefix); ref for the contributors compare
	Changes     []*changes.Change
	Headline    string
	Prerelease  bool
	Commit      string
	Version     string
	Root        string
	EnrichPR    bool
}

func mdText(text string) *MDText {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &MDText{Format: "markdown", Text: text}
}

func BuildData(p BuildParams) *ReleaseData {
	data := &ReleaseData{
		SchemaVersion: schemaVersion,
		Version:       p.Version,
		Tag:           p.Tag,
		Prerelease:    p.Prerelease,
		ReleasedAt:    time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		Commit:        p.Commit,
		Headline:      mdText(p.Headline),
	}
	if p.Previous != "" {
		prev := p.Previous
		data.PreviousVersion = &prev
	}

	for _, c := range p.Changes {
		entry := ChangeEntry{
			ID:          c.Slug(),
			Type:        c.Bump,
			Breaking:    c.Breaking,
			Categories:  c.Categories,
			Title:       MDText{Format: "markdown", Text: c.Title},
			Body:        mdText(c.Body),
			ReleaseNote: mdText(c.ReleaseNote),
			Notices:     make([]NoticeEntry, len(c.Notices)),
		}
		for i, n := range c.Notices {
			entry.Notices[i] = NoticeEntry{Level: n.Level, Message: n.Message}
		}
		switch {
		case c.PRExplicitNone:
			// explicit `pr: null` in frontmatter suppresses the PR link.
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

func WriteNotes(path string, data *ReleaseData, githubRepo string, maintainers map[string]bool) error {
	content := RenderChangelog(data, githubRepo, maintainers)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", path)
	return nil
}

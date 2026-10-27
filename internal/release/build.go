package release

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	GithubRepo  string // e.g. "OpenShock/Firmware" from GITHUB_REPOSITORY
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

var titlePRSuffix = regexp.MustCompile(`\s*\(#(\d+)\)$`)

// splitTitlePR strips a trailing "(#123)" from title and returns the number, so
// the renderers can add the PR reference once (and linked) instead of twice.
// Returns the title unchanged and 0 when there is no such suffix.
func splitTitlePR(title string) (string, int) {
	m := titlePRSuffix.FindStringSubmatchIndex(title)
	if m == nil {
		return title, 0
	}
	n, err := strconv.Atoi(title[m[2]:m[3]])
	if err != nil {
		return title, 0
	}
	return title[:m[0]], n
}

// uncommitted sorts change files that are not committed yet after all committed ones.
const uncommitted = math.MaxInt64

func BuildData(p BuildParams) *ReleaseData {
	data := &ReleaseData{
		SchemaVersion: schemaVersion,
		Repository:    parseRepository(p.GithubRepo),
		Version:       p.Version,
		Tag:           p.Tag,
		Prerelease:    p.Prerelease,
		ReleasedAt:    time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		Commit:        p.Commit,
		Headline:      strings.TrimSpace(p.Headline),
	}
	if p.Previous != "" {
		prev := p.Previous
		data.PreviousVersion = &prev
	}
	if p.PreviousTag != "" {
		data.PreviousTag = p.PreviousTag
	}

	// addedAt orders entries by when their change file was committed.
	addedAt := make([]int64, 0, len(p.Changes))

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

		var origin git.ChangeFileOrigin
		committed := false
		if p.Root != "" {
			origin, committed = git.ChangeFileAdded(p.Root, c.Filename)
		}

		// PR sources, most authoritative first. A "(#123)" written into the
		// title is moved into PR so it is not rendered twice.
		title, titlePR := splitTitlePR(c.Title)
		switch {
		case c.PRExplicitNone:
			// explicit `pr: null` suppresses the PR link; keep the title as authored
		case c.PR != nil:
			pr := *c.PR
			entry.PR = &pr
			if titlePR == pr {
				entry.Title = title
			}
		case titlePR != 0:
			entry.Title = title
			entry.PR = &titlePR
		case committed && git.PRFromSubject(origin.Subject) != 0:
			n := git.PRFromSubject(origin.Subject)
			entry.PR = &n
		case p.EnrichPR && committed:
			if n := git.PRForCommit(p.Root, origin.SHA); n != 0 {
				entry.PR = &n
			}
		}

		data.Changes = append(data.Changes, entry)
		if committed {
			addedAt = append(addedAt, origin.Time)
		} else {
			addedAt = append(addedAt, uncommitted)
		}
	}

	// Breaking changes first, then in the order they were committed. Ties (and
	// all entries when there is no git history) keep the filename order.
	order := make([]int, len(data.Changes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ea, eb := data.Changes[order[a]], data.Changes[order[b]]
		if ea.Breaking != eb.Breaking {
			return ea.Breaking
		}
		return addedAt[order[a]] < addedAt[order[b]]
	})
	if len(order) > 1 {
		sorted := make([]ChangeEntry, len(order))
		for i, idx := range order {
			sorted[i] = data.Changes[idx]
		}
		data.Changes = sorted
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

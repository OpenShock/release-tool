package changes

import (
	"fmt"
	"strings"
)

// Severity of a lint finding. Errors fail `check` and releases; warnings are
// reported but do not block.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is one lint result for a change file.
type Finding struct {
	File     string   `json:"file"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

func (f Finding) String() string {
	return fmt.Sprintf("%s: %s: %s", f.File, f.Severity, f.Message)
}

// notPastTense are words ending in "ed" that are not past-tense verbs.
var notPastTense = map[string]bool{"embed": true, "shed": true, "shred": true}

// pastTense reports whether word looks like a past-tense verb ("Added",
// "Fixed"), the tense change titles should not use.
func pastTense(word string) bool {
	w := strings.ToLower(strings.Trim(word, "`*_:"))
	return len(w) > 3 && strings.HasSuffix(w, "ed") && !strings.HasSuffix(w, "eed") && !notPastTense[w]
}

// Lint checks pending changes for content problems that parsing accepts but
// that make release notes worse. It returns errors for omissions readers get
// hurt by and warnings for style.
func Lint(ch []*Change) []Finding {
	var out []Finding
	for _, c := range ch {
		if c.Breaking && strings.TrimSpace(c.ReleaseNote) == "" && len(c.Notices) == 0 {
			out = append(out, Finding{c.Filename, SeverityError,
				"breaking change must tell users what to do: add a `## Release Note` or a `## Notices` entry"})
		}
		if fields := strings.Fields(c.Title); len(fields) > 0 && pastTense(fields[0]) {
			out = append(out, Finding{c.Filename, SeverityWarning,
				fmt.Sprintf("title starts with %q; use the imperative mood (\"Add\", \"Fix\"), not the past tense", fields[0])})
		}
		if c.Kind == "safety" && strings.TrimSpace(c.ReleaseNote) == "" {
			out = append(out, Finding{c.Filename, SeverityWarning,
				"safety change has no `## Release Note`; describe the new behaviour for users"})
		}
	}
	return out
}

// HasErrors reports whether any finding is an error.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

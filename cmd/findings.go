package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
)

// reportFindings prints lint findings to stderr, as GitHub annotations when
// running in Actions so they show up on the offending file.
func reportFindings(findings []changes.Finding) {
	annotate := os.Getenv("GITHUB_ACTIONS") == "true"
	for _, f := range findings {
		if annotate {
			fmt.Fprintf(os.Stderr, "::%s file=%s::%s\n", f.Severity, path.Join(changes.Dir, f.File), escapeAnnotation(f.Message))
		} else {
			fmt.Fprintln(os.Stderr, f.String())
		}
	}
}

// escapeAnnotation encodes the characters GitHub workflow commands reserve.
func escapeAnnotation(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// lintPending reports lint findings for ch and fails when any is an error, so
// a release never ships notes with a known omission.
func lintPending(ch []*changes.Change) error {
	findings := changes.Lint(ch)
	reportFindings(findings)
	if changes.HasErrors(findings) {
		return fmt.Errorf("change files have lint errors")
	}
	return nil
}

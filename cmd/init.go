package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise .changes/ in the target repo",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const changesReadme = `# Change Files

Each ` + "`" + `.md` + "`" + ` file in this directory describes one pending change that will be
included in the next release.

## Format

` + "```" + `
---
type: minor        # major | minor | patch
breaking: false    # optional; major defaults to true
categories: []     # optional list of labels
---
Title of the change (first line, required)

Optional longer description in Markdown.

## Release Note
Optional consumer-facing note included in release.json but not the changelog.

## Notices
- warning: something users must know before upgrading
- info: optional note or migration step
` + "```" + `

## File naming

Name the file after the change (e.g. ` + "`" + `add-user-auth.md` + "`" + `).
Run ` + "`" + `release-tool new "<title>" --type minor` + "`" + ` to generate one automatically.

## Special files

- ` + "`" + `_headline.md` + "`" + ` - optional release headline shown at the top of the changelog entry
`

func runInit(_ *cobra.Command, _ []string) error {
	root := projectRoot()
	dir := filepath.Join(root, changes.Dir)

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists", changes.Dir)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", changes.Dir, err)
	}

	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(changesReadme), 0644); err != nil {
		return fmt.Errorf("writing README: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Initialised %s/\n", changes.Dir)
	return nil
}

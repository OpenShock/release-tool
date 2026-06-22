package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/spf13/cobra"
)

var (
	initActionRef   string
	initBranches    []string
	initDevelop     []string
	initTagPrefix   string
	initNoWorkflows bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise .changes/ and GitHub Actions workflows in the target repo",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&initActionRef, "action-ref", "OpenShock/release-tool@main", "Action ref to use in generated workflows (pin to a SHA for production)")
	initCmd.Flags().StringSliceVar(&initBranches, "branches", []string{"master"}, "Release branches (stable first, then prerelease)")
	initCmd.Flags().StringSliceVar(&initDevelop, "develop", nil, "Branches that build without tagging (release:none, sha:true)")
	initCmd.Flags().StringVar(&initTagPrefix, "tag-prefix", "", "Tag prefix (e.g. 'v')")
	initCmd.Flags().BoolVar(&initNoWorkflows, "no-workflows", false, "Skip creating .github/workflows/ files")
}

const changesReadme = `# Change Files

Each ` + "`" + `.md` + "`" + ` file in this directory describes one pending change that will be
included in the next release.

## Format

` + "```" + `
---
kind: added       # added | changed | deprecated | removed | fixed | security | safety | chore
breaking: false   # optional; true forces a major semver bump
mandatory: false  # optional; true means this version must be installed before newer ones
---
Technical title (required, one line — appears in CHANGELOG and GitHub Release)

## Release Note
User-facing title line (appears in release.json)
Additional description lines shown in the GitHub Release and release.json.

## Notices
- warning: something users must know before upgrading
- info: optional note or migration step
` + "```" + `

## Semver derivation

- ` + "`" + `breaking: true` + "`" + ` -> major bump
- ` + "`" + `kind: fixed|security|safety|chore` + "`" + ` -> patch bump
- everything else -> minor bump

## File naming

Name the file after the change (e.g. ` + "`" + `add-user-auth.md` + "`" + `).
Run ` + "`" + `release-tool new "<title>" --kind added` + "`" + ` to generate one automatically.

## Special files

- ` + "`" + `_headline.md` + "`" + ` - optional release headline shown at the top of the GitHub Release body
`

func writeIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "Skipping %s (already exists)\n", path)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Created %s\n", path)
	return nil
}

func runInit(_ *cobra.Command, _ []string) error {
	root := projectRoot()
	dir := filepath.Join(root, changes.Dir)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", changes.Dir, err)
	}

	if err := writeIfMissing(filepath.Join(dir, "README.md"), changesReadme); err != nil {
		return err
	}
	if err := writeIfMissing(filepath.Join(dir, "config.json"), buildConfigJSON(initTagPrefix, initBranches, initDevelop)); err != nil {
		return err
	}

	if !initNoWorkflows {
		allBranches := append(append([]string{}, initBranches...), initDevelop...)
		if err := writeWorkflows(root, initActionRef, allBranches); err != nil {
			return err
		}
	}

	return nil
}

func buildConfigJSON(tagPrefix string, branches, develop []string) string {
	var branchEntries []string
	for i, b := range branches {
		var entry string
		if i == 0 {
			entry = fmt.Sprintf(`    %q: { "release": "stable" }`, b)
		} else {
			entry = fmt.Sprintf(`    %q: { "release": "prerelease", "label": %q }`, b, b)
		}
		branchEntries = append(branchEntries, entry)
	}
	for _, b := range develop {
		entry := fmt.Sprintf(`    %q: { "release": "none", "label": %q, "sha": true }`, b, b)
		branchEntries = append(branchEntries, entry)
	}

	return fmt.Sprintf(`{
  "tag_prefix": %q,
  "branches": {
%s
  }
}
`, tagPrefix, strings.Join(branchEntries, ",\n"))
}

func writeWorkflows(root, actionRef string, branches []string) error {
	wfDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		return fmt.Errorf("creating .github/workflows: %w", err)
	}

	branchList := `[` + strings.Join(branches, ", ") + `]`

	checkYML := fmt.Sprintf(`on:
  pull_request:
    branches: %s
    types: [opened, reopened, synchronize, ready_for_review, labeled, unlabeled]

name: check-changes

permissions:
  contents: read

jobs:
  check:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    if: >-
      !github.event.pull_request.draft &&
      !contains(github.event.pull_request.labels.*.name, 'no-changelog')
    steps:
      - uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6.0.3
        with:
          fetch-depth: 0

      - name: Run change file check
        uses: %s
        with:
          mode: check
          base-ref: ${{ github.event.pull_request.base.ref }}
          base-sha: ${{ github.event.pull_request.base.sha }}
          pr-number: ${{ github.event.pull_request.number }}

      - name: Upload verdict
        if: always()
        uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: release-check
          path: release-check.json
          if-no-files-found: warn
`, branchList, actionRef)

	commentYML := `on:
  workflow_run:
    workflows: [check-changes]
    types: [completed]

name: pr-check-comment

permissions:
  pull-requests: write

jobs:
  comment:
    runs-on: ubuntu-latest
    if: github.event.workflow_run.event == 'pull_request'
    steps:
      - name: Download verdict
        id: download
        continue-on-error: true
        uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1
        with:
          name: release-check
          run-id: ${{ github.event.workflow_run.id }}
          github-token: ${{ secrets.GITHUB_TOKEN }}

      - name: Post, update, or remove sticky comment
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_REPO: ${{ github.repository }}
          DOWNLOAD_OK: ${{ steps.download.outcome == 'success' }}
          FALLBACK_PR: ${{ github.event.workflow_run.pull_requests[0].number }}
        run: |
          if [ "$DOWNLOAD_OK" = "true" ]; then
            STATE=$(jq -r '.state' release-check.json)
            PR=$(jq -r '.pr' release-check.json)
            BODY=$(jq -r '.body' release-check.json)
          else
            STATE="skip"
            PR="$FALLBACK_PR"
            BODY=""
          fi

          if [ -z "$PR" ] || [ "$PR" = "0" ] || [ "$PR" = "null" ]; then
            echo "No PR number, nothing to do"
            exit 0
          fi

          EXISTING=$(gh api "repos/$GH_REPO/issues/$PR/comments" \
            --jq '[.[] | select(.body | contains("<!-- release-tool-check -->"))] | first | .id // empty')

          if [ "$STATE" = "skip" ]; then
            if [ -n "$EXISTING" ]; then
              gh api --method DELETE "repos/$GH_REPO/issues/comments/$EXISTING"
              echo "Deleted stale comment $EXISTING on PR #$PR"
            fi
          elif [ -n "$EXISTING" ]; then
            gh api --method PATCH "repos/$GH_REPO/issues/comments/$EXISTING" \
              --field body="$BODY"
          else
            gh api --method POST "repos/$GH_REPO/issues/$PR/comments" \
              --field body="$BODY"
          fi
`

	if err := writeIfMissing(filepath.Join(wfDir, "check-changes.yml"), checkYML); err != nil {
		return err
	}
	if err := writeIfMissing(filepath.Join(wfDir, "pr-check-comment.yml"), commentYML); err != nil {
		return err
	}

	return nil
}

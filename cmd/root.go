package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/spf13/cobra"
)

var (
	dryRun          bool
	output          string
	notes           string
	prereleaseLabel string
	gitSHA          bool
	rootDir         string
)

// releaseOptions is the resolved input to a single release/prerelease run. It is
// built once from flags (and branch config) in each command's RunE and passed
// down explicitly, so commands never mutate shared package globals to talk to
// each other.
type releaseOptions struct {
	dryRun          bool
	output          string
	notes           string
	prereleaseLabel string
	gitSHA          bool
	noTag           bool
}

var rootCmd = &cobra.Command{
	Use:   "release-tool",
	Short: "OpenShock release tool - manages .changes files and versioned releases",
	// Errors are validation/runtime failures, not usage mistakes; don't dump
	// the help text on every error.
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootDir, "root", "", "Root directory of the target repo (defaults to cwd)")
}

func projectRoot() string {
	if rootDir != "" {
		return rootDir
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get working directory:", err)
		os.Exit(1)
	}
	return root
}

// appendGitHubOutput appends key=value step outputs to GITHUB_OUTPUT if the env
// var is set. Values here are short, newline-free version/tag strings, so the
// plain key=value form is sufficient (no heredoc delimiter needed).
func appendGitHubOutput(outputs map[string]string) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open GITHUB_OUTPUT: %v\n", err)
		return
	}
	defer f.Close()
	for k, v := range outputs {
		if _, err := fmt.Fprintf(f, "%s=%s\n", k, v); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write GITHUB_OUTPUT: %v\n", err)
			return
		}
	}
}

// writeGitHubOutputs writes the release/prerelease step outputs. version is the
// full computed version string (the tag without prefix), exposed even on the
// no-tag path where tag is empty; previous is the prior stable base version.
func writeGitHubOutputs(tag, version, previous string, prerelease bool) {
	pre := "false"
	if prerelease {
		pre = "true"
	}
	appendGitHubOutput(map[string]string{
		"tag":              tag,
		"version":          version,
		"previous-version": previous,
		"prerelease":       pre,
		"skip":             "false",
	})
}

// writeStatusOutputs writes the status step outputs: the next base version
// (MAJOR.MINOR.PATCH, no prerelease label), the prior stable base, and the
// computed bump level. Works on any branch since it has no branch-config input.
func writeStatusOutputs(nextVersion, previous, bump string) {
	appendGitHubOutput(map[string]string{
		"next-version":     nextVersion,
		"previous-version": previous,
		"bump":             bump,
		"skip":             "false",
	})
}

// enrichment derives the GitHub-enrichment inputs shared by the stable and rc
// commands: the literal previous tag (ref for the contributors compare) and the
// maintainer exclusion set. Maintainers are fetched only outside dry-run, to
// avoid network calls during previews.
func enrichment(root string, cfg *changes.Config, latest string, dryRun bool) (prevTag string, maintainers map[string]bool) {
	if latest != "" {
		prevTag = cfg.TagPrefix + latest
	}
	// Always seed from the configured maintainer list so the footer excludes
	// them even when gh can't return collaborators (the default-token case).
	maintainers = map[string]bool{}
	for _, m := range cfg.Maintainers {
		maintainers[strings.ToLower(m)] = true
	}
	if !dryRun {
		for m := range git.Maintainers(root) {
			maintainers[m] = true
		}
	}
	return prevTag, maintainers
}

func writeGitHubOutputSkip() {
	appendGitHubOutput(map[string]string{"skip": "true"})
}

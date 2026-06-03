package cmd

import (
	"fmt"
	"os"

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

var rootCmd = &cobra.Command{
	Use:   "release-tool",
	Short: "OpenShock release tool - manages .changes files and versioned releases",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview without making changes")
	rootCmd.PersistentFlags().StringVar(&output, "output", "release.json", "Path to write release.json")
	rootCmd.PersistentFlags().StringVar(&notes, "notes", "", "Path to write markdown release notes")
	rootCmd.PersistentFlags().StringVar(&prereleaseLabel, "prerelease-label", "", "Override prerelease label (e.g. rc, alpha, beta)")
	rootCmd.PersistentFlags().BoolVar(&gitSHA, "git-sha", false, "Append git short SHA as build metadata (+g<sha>)")
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

// writeGitHubOutputs writes step outputs to GITHUB_OUTPUT if the env var is set.
func writeGitHubOutputs(tag string, prerelease bool) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	pre := "false"
	if prerelease {
		pre = "true"
	}
	fmt.Fprintf(f, "tag=%s\nprerelease=%s\nskip=false\n", tag, pre)
}

func writeGitHubOutputSkip() {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "skip=true")
}

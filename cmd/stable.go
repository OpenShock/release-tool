package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/spf13/cobra"
)

var stableCmd = &cobra.Command{
	Use:   "stable",
	Short: "Promote to stable release, consume .changes files, update CHANGELOG.md",
	RunE:  runStable,
}

func init() {
	rootCmd.AddCommand(stableCmd)
}

func runStable(_ *cobra.Command, _ []string) error {
	root := projectRoot()

	ch, err := changes.Read(root)
	if err != nil {
		return err
	}
	if len(ch) == 0 {
		fmt.Println("No pending changes, nothing to release.")
		writeGitHubOutputSkip()
		return nil
	}

	cfg, err := changes.ReadConfig(root)
	if err != nil {
		return err
	}

	latest, err := git.LatestStableTag(root, cfg.TagPrefix)
	if err != nil {
		return err
	}
	base, err := release.ComputeNext(ch, latest)
	if err != nil {
		return err
	}

	tag := cfg.TagPrefix + base
	isPrerelease := prereleaseLabel != ""
	if isPrerelease {
		num, err := git.LatestPrereleaseNumber(root, cfg.TagPrefix+base, prereleaseLabel)
		if err != nil {
			return err
		}
		tag = fmt.Sprintf("%s%s-%s.%d", cfg.TagPrefix, base, prereleaseLabel, num+1)
	}

	commit, err := git.CurrentCommit(root)
	if err != nil {
		return err
	}

	data := release.BuildData(release.BuildParams{
		Tag:        tag,
		Previous:   latest,
		Changes:    ch,
		Headline:   changes.ReadHeadline(root),
		Prerelease: isPrerelease,
		Commit:     commit,
		Version:    base,
		Root:       root,
		EnrichPR:   !dryRun,
	})

	entry := release.RenderChangelog(data, os.Getenv("GITHUB_REPOSITORY"))

	if dryRun {
		fmt.Fprintf(os.Stderr, "Would create tag: %s\n", tag)
		fmt.Fprintf(os.Stderr, "\nChangelog entry:\n%s\n", entry)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	if err := release.WriteJSON(output, data); err != nil {
		return err
	}
	if notes != "" {
		if err := release.WriteNotes(notes, data, os.Getenv("GITHUB_REPOSITORY")); err != nil {
			return err
		}
	}

	changelogPath := filepath.Join(root, "CHANGELOG.md")
	existing, _ := os.ReadFile(changelogPath)
	if err := os.WriteFile(changelogPath, []byte(entry+"\n"+string(existing)), 0644); err != nil {
		return fmt.Errorf("writing CHANGELOG.md: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Updated CHANGELOG.md")

	removed := 0
	for _, c := range ch {
		if err := os.Remove(filepath.Join(root, changes.Dir, c.Filename)); err == nil {
			removed++
		}
	}
	if err := os.Remove(filepath.Join(root, changes.Dir, changes.HeadlineFile)); err == nil {
		fmt.Fprintln(os.Stderr, "Removed .changes/_headline.md")
	}
	fmt.Fprintf(os.Stderr, "Removed %d change files\n", removed)

	if err := git.Add(root, "CHANGELOG.md", changes.Dir); err != nil {
		return err
	}
	if err := git.Commit(root, "chore: release "+tag); err != nil {
		return err
	}
	if err := git.CreateTag(root, tag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created tag: %s\n", tag)
	writeGitHubOutputs(tag, isPrerelease)
	return nil
}

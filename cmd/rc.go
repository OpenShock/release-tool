package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/spf13/cobra"
)

var rcCmd = &cobra.Command{
	Use:   "rc",
	Short: "Create a prerelease tag from pending changes without consuming them",
	RunE:  runRC,
}

func init() {
	rootCmd.AddCommand(rcCmd)
}

func runRC(_ *cobra.Command, _ []string) error {
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
	if prereleaseLabel != "" {
		num, err := git.LatestPrereleaseNumber(root, cfg.TagPrefix+base, prereleaseLabel)
		if err != nil {
			return err
		}
		tag = fmt.Sprintf("%s%s-%s.%d", cfg.TagPrefix, base, prereleaseLabel, num+1)
	}
	if gitSHA {
		sha, err := git.ShortSHA(root)
		if err != nil {
			return err
		}
		tag += "+g" + sha
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
		Prerelease: true,
		Commit:     commit,
		Version:    base,
		Root:       root,
		EnrichPR:   !dryRun,
	})

	if dryRun {
		fmt.Fprintf(os.Stderr, "Would create tag: %s\n", tag)
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
	if err := git.CreateTag(root, tag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created tag: %s\n", tag)
	writeGitHubOutputs(tag, true)
	return nil
}

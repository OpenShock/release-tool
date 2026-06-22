package cmd

import (
	"fmt"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Create a release or prerelease based on branch config",
	RunE: func(_ *cobra.Command, _ []string) error {
		root := projectRoot()

		cfg, err := changes.ReadConfig(root)
		if err != nil {
			return err
		}

		branch, err := git.CurrentBranch(root)
		if err != nil {
			return err
		}

		bcfg, ok := cfg.Branches[branch]
		if !ok {
			return fmt.Errorf("branch %q is not listed in .changes/config.json", branch)
		}

		opts := releaseOptions{dryRun: dryRun, output: output, notes: notes}

		switch bcfg.Release {
		case changes.ReleaseModeStable:
			return runRelease(opts)
		case changes.ReleaseModePrerelease:
			opts.prereleaseLabel = bcfg.Label
			opts.gitSHA = bcfg.SHA
			return runPrerelease(opts)
		default: // none — write release.json but no git tag
			opts.noTag = true
			opts.prereleaseLabel = bcfg.Label
			opts.gitSHA = bcfg.SHA
			return runPrerelease(opts)
		}
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)
	releaseCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without making changes")
	releaseCmd.Flags().StringVar(&output, "output", "release.json", "Path to write release.json")
	releaseCmd.Flags().StringVar(&notes, "notes", "", "Path to write markdown release notes")
}

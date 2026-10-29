package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

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

		releasedAt, err := sourceDateEpoch()
		if err != nil {
			return err
		}

		opts := releaseOptions{dryRun: dryRun, output: output, notes: notes, version: version, releasedAt: releasedAt}
		if version != "" && bcfg.Release != changes.ReleaseModeStable {
			return fmt.Errorf("--version only applies to stable releases; branch %q is %q", branch, bcfg.Release)
		}

		switch bcfg.Release {
		case changes.ReleaseModeStable:
			return runRelease(opts)
		case changes.ReleaseModePrerelease:
			opts.prereleaseLabel = bcfg.Label
			opts.gitSHA = bcfg.SHA
			return runPrerelease(opts)
		default: // none: write release.json but no git tag
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
	releaseCmd.Flags().StringVar(&version, "version", "", "Release this MAJOR.MINOR.PATCH instead of the computed version (stable only; not below it)")
}

// sourceDateEpoch reads SOURCE_DATE_EPOCH (Unix seconds), the release time to
// record instead of now, so a release can be replayed byte for byte. Unset
// returns the zero time.
func sourceDateEpoch() (time.Time, error) {
	v := os.Getenv("SOURCE_DATE_EPOCH")
	if v == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("SOURCE_DATE_EPOCH %q is not a Unix timestamp", v)
	}
	return time.Unix(secs, 0), nil
}

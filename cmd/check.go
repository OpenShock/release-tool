package cmd

import (
	"fmt"
	"os"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/spf13/cobra"
)

var (
	checkBase    string
	checkAgainst string
	checkPR      int
	checkOut     string
	checkRequire bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate change files added by a pull request and write a verdict file",
	Long: "Evaluate the change files a pull request adds relative to its base branch and " +
		"write a verdict (ok, missing, invalid, or skip) for a downstream comment step. " +
		"The base branch is resolved via the branches map in .changes/config.json; a base " +
		"that is not a release branch yields skip.",
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringVar(&checkBase, "base", "", "PR base branch name (resolved via .changes/config.json branches)")
	checkCmd.Flags().StringVar(&checkAgainst, "against", "", "Git ref to diff against for added change files (defaults to origin/<base>)")
	checkCmd.Flags().IntVar(&checkPR, "pr", 0, "Pull request number, recorded in the verdict for the comment stage")
	checkCmd.Flags().StringVar(&checkOut, "out", "release-check.json", "Path to write the verdict JSON")
	checkCmd.Flags().BoolVar(&checkRequire, "require-changes", false, "Fail when the PR adds no change file (default: warn only)")
}

func runCheck(_ *cobra.Command, _ []string) error {
	root := projectRoot()

	cfg, err := changes.ReadConfig(root)
	if err != nil {
		return err
	}

	against := checkAgainst
	if against == "" {
		against = "origin/" + checkBase
	}

	v, err := release.RunCheck(release.CheckParams{
		Root:       root,
		BaseBranch: checkBase,
		Against:    against,
		PR:         checkPR,
		Config:     cfg,
	})
	if err != nil {
		return err
	}

	// Always write the verdict, even on a failing state, so the comment stage
	// can report it.
	if err := release.WriteVerdict(checkOut, v); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "check: %s (pr #%d) -> %s\n", v.State, v.PR, checkOut)
	writeCheckState(v.State)

	switch v.State {
	case release.StateInvalid:
		return fmt.Errorf("change file check failed: invalid format")
	case release.StateMissing:
		if checkRequire {
			return fmt.Errorf("change file check failed: no change file added")
		}
	}
	return nil
}

func writeCheckState(state release.CheckState) {
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
	if _, err := fmt.Fprintf(f, "state=%s\n", state); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write GITHUB_OUTPUT: %v\n", err)
	}
}

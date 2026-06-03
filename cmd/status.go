package cmd

import (
	"fmt"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show pending changes and next version",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(_ *cobra.Command, _ []string) error {
	root := projectRoot()

	ch, err := changes.Read(root)
	if err != nil {
		return err
	}
	if len(ch) == 0 {
		fmt.Println("No pending changes.")
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
	fmt.Printf("Latest stable tag: %s\n", orNone(latest))

	highest := release.HighestBump(ch)
	if latest != "" {
		maj, min, pat, err := release.ParseVersion(latest)
		if err != nil {
			return err
		}
		maj, min, pat = release.BumpVersion(maj, min, pat, highest)
		fmt.Printf("Bump level:        %s\n", highest)
		fmt.Printf("Next version:      %d.%d.%d\n", maj, min, pat)
	}
	fmt.Println()

	for _, c := range ch {
		var flags []string
		if c.Breaking {
			flags = append(flags, "breaking")
		}
		if len(c.Categories) > 0 {
			flags = append(flags, "cat:"+strings.Join(c.Categories, ","))
		}
		if c.ReleaseNote != "" {
			flags = append(flags, "release_note")
		}
		if len(c.Notices) > 0 {
			flags = append(flags, fmt.Sprintf("%d notices", len(c.Notices)))
		}
		extra := ""
		if len(flags) > 0 {
			extra = "  (" + strings.Join(flags, ", ") + ")"
		}
		fmt.Printf("  [%s] %s%s  <- %s\n", c.Bump, c.Title, extra, c.Filename)
	}

	headline := changes.ReadHeadline(root)
	if headline != "" {
		fmt.Printf("\nHeadline (%d chars) from .changes/_headline.md\n", len(headline))
	}

	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/spf13/cobra"
)

var allowEmpty bool

var rcCmd = &cobra.Command{
	Use:   "rc",
	Short: "Create a prerelease tag from pending changes without consuming them",
	RunE:  runRC,
}

func init() {
	rootCmd.AddCommand(rcCmd)
	rcCmd.Flags().BoolVar(&allowEmpty, "allow-empty", false,
		"Create a tag even when no .changes files exist, using the last stable version as the base (no bump)")
}

// sincePatterns returns tag-matching regexps ordered by priority for the given
// label. develop looks back to the last develop, beta, or stable tag; beta
// looks back to the last beta or stable tag; other labels return nil (read all).
func sincePatterns(prefix, label string) []*regexp.Regexp {
	q := regexp.QuoteMeta(prefix)
	stable := regexp.MustCompile(`^` + q + `\d+\.\d+\.\d+$`)
	switch label {
	case "develop":
		return []*regexp.Regexp{
			regexp.MustCompile(`^` + q + `\d+\.\d+\.\d+-develop\b`),
			regexp.MustCompile(`^` + q + `\d+\.\d+\.\d+-beta\.\d+`),
			stable,
		}
	case "beta":
		return []*regexp.Regexp{
			regexp.MustCompile(`^` + q + `\d+\.\d+\.\d+-beta\.\d+`),
			stable,
		}
	default:
		return nil
	}
}

func gatherChangesSince(root string, patterns []*regexp.Regexp) ([]*changes.Change, error) {
	ref, err := git.LatestTagMatching(root, patterns)
	if err != nil {
		return nil, err
	}
	filenames, err := git.ChangedChangeFilesSinceRef(root, ref)
	if err != nil {
		return nil, err
	}
	return changes.ReadSubset(root, filenames)
}

func runRC(_ *cobra.Command, _ []string) error {
	root := projectRoot()

	cfg, err := changes.ReadConfig(root)
	if err != nil {
		return err
	}

	patterns := sincePatterns(cfg.TagPrefix, prereleaseLabel)

	var ch []*changes.Change
	if patterns != nil {
		ch, err = gatherChangesSince(root, patterns)
	} else {
		ch, err = changes.Read(root)
	}
	if err != nil {
		return err
	}

	latest, err := git.LatestStableTag(root, cfg.TagPrefix)
	if err != nil {
		return err
	}

	var base string
	if len(ch) == 0 {
		if !allowEmpty {
			fmt.Println("No pending changes, nothing to release.")
			writeGitHubOutputSkip()
			return nil
		}
		// No changes: use last stable as base, no bump.
		base = latest
		if base == "" {
			base = "0.0.0"
		}
	} else {
		base, err = release.ComputeNext(ch, latest)
		if err != nil {
			return err
		}
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

	prevTag, maintainers := enrichment(root, cfg, latest)

	data := release.BuildData(release.BuildParams{
		Tag:         tag,
		Previous:    latest,
		PreviousTag: prevTag,
		Changes:     ch,
		Headline:    changes.ReadHeadline(root),
		Prerelease:  true,
		Commit:      commit,
		Version:     base,
		Root:        root,
		EnrichPR:    !dryRun,
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
		if err := release.WriteNotes(notes, data, os.Getenv("GITHUB_REPOSITORY"), maintainers); err != nil {
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

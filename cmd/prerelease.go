package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/OpenShock/release-tool/internal/versionfile"
	"github.com/spf13/cobra"
)

// commitVersionFiles sets the configured version files to version and commits
// them as the release commit for tag. Nothing is committed when no file
// changes. On failure the working tree is reset to where it started.
func commitVersionFiles(root string, cfg *changes.Config, version, tag string) error {
	if len(cfg.VersionFiles) == 0 {
		return nil
	}
	if !git.IdentityConfigured(root) {
		return fmt.Errorf("no git committer identity configured; set user.name and user.email (or GIT_AUTHOR_*/GIT_COMMITTER_* env) before releasing")
	}
	head, err := git.CurrentCommit(root)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		if rbErr := git.ResetHard(root, head); rbErr != nil {
			return fmt.Errorf("%w (rollback to %s also failed: %v)", cause, head, rbErr)
		}
		return cause
	}
	changed, err := versionfile.Update(root, cfg.VersionFiles, version)
	if err != nil {
		return rollback(err)
	}
	if len(changed) == 0 {
		return nil
	}
	if err := git.Add(root, changed...); err != nil {
		return rollback(err)
	}
	if err := git.Commit(root, "chore: release "+tag); err != nil {
		return rollback(err)
	}
	fmt.Fprintf(os.Stderr, "Set version %s in %s\n", version, strings.Join(changed, ", "))
	return nil
}

var prereleaseCmd = &cobra.Command{
	Use:   "prerelease",
	Short: "Create a prerelease tag from pending changes without consuming them",
	RunE: func(_ *cobra.Command, _ []string) error {
		return runPrerelease(releaseOptions{
			dryRun:          dryRun,
			output:          output,
			notes:           notes,
			prereleaseLabel: prereleaseLabel,
			gitSHA:          gitSHA,
		})
	},
}

func init() {
	rootCmd.AddCommand(prereleaseCmd)
	prereleaseCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without making changes")
	prereleaseCmd.Flags().StringVar(&output, "output", "release.json", "Path to write release.json")
	prereleaseCmd.Flags().StringVar(&notes, "notes", "", "Path to write markdown release notes")
	prereleaseCmd.Flags().StringVar(&prereleaseLabel, "prerelease-label", "", "Override prerelease label (e.g. rc, alpha, beta)")
	prereleaseCmd.Flags().BoolVar(&gitSHA, "git-sha", false, "Append git short SHA as build metadata (+g<sha>)")
}

func runPrerelease(opts releaseOptions) error {
	root := projectRoot()

	cfg, err := changes.ReadConfig(root)
	if err != nil {
		return err
	}

	latest, err := git.LatestStableTag(root, cfg.TagPrefix)
	if err != nil {
		return err
	}

	var ch []*changes.Change
	if latest == "" {
		ch, err = changes.Read(root)
	} else {
		var filenames []string
		filenames, err = git.ChangedChangeFilesSinceRef(root, cfg.TagPrefix+latest)
		if err != nil {
			return err
		}
		ch, err = changes.ReadSubset(root, filenames)
	}
	if err != nil {
		return err
	}
	if len(ch) == 0 {
		fmt.Println("No pending changes, nothing to release.")
		writeGitHubOutputSkip()
		return nil
	}
	if err := lintPending(ch); err != nil {
		return err
	}
	knownIssues, err := changes.ReadKnownIssues(root)
	if err != nil {
		return err
	}

	base, err := release.ComputeNext(ch, latest)
	if err != nil {
		return err
	}

	tag := cfg.TagPrefix + base
	switch {
	case opts.prereleaseLabel != "" && opts.gitSHA:
		// SHA is the unique identifier: no .N counter, no tag lookup.
		sha, err := git.ShortSHA(root)
		if err != nil {
			return err
		}
		tag = fmt.Sprintf("%s%s-%s+g%s", cfg.TagPrefix, base, opts.prereleaseLabel, sha)
	case opts.prereleaseLabel != "":
		num, err := git.LatestPrereleaseNumber(root, cfg.TagPrefix+base, opts.prereleaseLabel)
		if err != nil {
			return err
		}
		tag = fmt.Sprintf("%s%s-%s.%d", cfg.TagPrefix, base, opts.prereleaseLabel, num+1)
	}

	// The SHA-based and label-less tags are deterministic from the commit, so a
	// CI re-run on the same commit would recompute an existing tag. Detect that
	// before writing any outputs and treat it as already-released, rather than
	// failing at CreateTag after release.json/notes were already produced.
	if !opts.dryRun && !opts.noTag {
		if exists, err := git.TagExists(root, tag); err != nil {
			return err
		} else if exists {
			fmt.Printf("Tag %s already exists, nothing to release.\n", tag)
			writeGitHubOutputSkip()
			return nil
		}
		if !opts.gitSHA {
			if err := validateVersionFiles(root, cfg); err != nil {
				return err
			}
		}
	}

	commit, err := git.CurrentCommit(root)
	if err != nil {
		return err
	}

	prevTag, maintainers := enrichment(root, cfg, latest, opts.dryRun)

	// A numbered prerelease (rc.N) lists what is new since the previous one, so
	// testers know what to look at. SHA builds have no meaningful predecessor.
	var prevPre string
	var newSince []string
	if opts.prereleaseLabel != "" && !opts.gitSHA {
		prevPre = git.PreviousPrereleaseTag(root, cfg.TagPrefix, opts.prereleaseLabel, prevTag, tag)
		if prevPre != "" {
			if newSince, err = git.ChangedChangeFilesSinceRef(root, prevPre); err != nil {
				return err
			}
		}
	}

	githubRepo := os.Getenv("GITHUB_REPOSITORY")

	data := release.BuildData(release.BuildParams{
		Tag:                tag,
		Previous:           latest,
		PreviousTag:        prevTag,
		Changes:            ch,
		Headline:           changes.ReadHeadline(root),
		KnownIssues:        knownIssues,
		PreviousPrerelease: prevPre,
		NewSincePrerelease: newSince,
		Prerelease:         true,
		Commit:             commit,
		Version:            base,
		Root:               root,
		EnrichPR:           !opts.dryRun,
		GithubRepo:         githubRepo,
		ReleasedAt:         opts.releasedAt,
	})

	if opts.dryRun {
		fmt.Fprintf(os.Stderr, "Would create tag: %s\n", tag)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	if err := release.WriteJSON(opts.output, data); err != nil {
		return err
	}
	if opts.notes != "" {
		if err := release.WriteNotes(opts.notes, data, notesOptions(cfg, maintainers)); err != nil {
			return err
		}
	}
	if opts.noTag {
		fmt.Fprintf(os.Stderr, "Version: %s (no tag)\n", tag)
		writeGitHubOutputs("", tag, latest, true)
		return nil
	}
	// A SHA build's tag names the commit it was cut from, so it gets no
	// release commit; every other prerelease records its version in one.
	if !opts.gitSHA {
		if err := commitVersionFiles(root, cfg, strings.TrimPrefix(tag, cfg.TagPrefix), tag); err != nil {
			return err
		}
	}
	if err := git.CreateTag(root, tag); err != nil {
		if rbErr := git.ResetHard(root, commit); rbErr != nil {
			return fmt.Errorf("%w (rollback to %s also failed: %v)", err, commit, rbErr)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "Created tag: %s\n", tag)
	writeGitHubOutputs(tag, tag, latest, true)
	return nil
}

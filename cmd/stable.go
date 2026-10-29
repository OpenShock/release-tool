package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
	"github.com/OpenShock/release-tool/internal/versionfile"
)

// validateVersionFiles rejects bad version_files entries before a release
// touches the working tree. Each file must be tracked: a rollback (git reset
// --hard) cannot restore an untracked or ignored file, and the release commit
// should not add one.
func validateVersionFiles(root string, cfg *changes.Config) error {
	for _, f := range cfg.VersionFiles {
		if err := f.Validate(); err != nil {
			return err
		}
		if !git.Tracked(root, f.Path) {
			return fmt.Errorf("version_files: %s is not tracked by git", f.Path)
		}
	}
	return nil
}

func runRelease(opts releaseOptions) error {
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

	if err := lintPending(ch); err != nil {
		return err
	}
	headline := changes.ReadHeadline(root)
	if headline == "" {
		fmt.Fprintf(os.Stderr, "warning: no %s/%s; a stable release should say what matters most in it\n", changes.Dir, changes.HeadlineFile)
	}
	knownIssues, err := changes.ReadKnownIssues(root)
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
	if opts.version != "" {
		if base, err = release.OverrideVersion(opts.version, cfg.TagPrefix, base); err != nil {
			return err
		}
	}

	tag := cfg.TagPrefix + base

	commit, err := git.CurrentCommit(root)
	if err != nil {
		return err
	}

	prevTag, maintainers := enrichment(root, cfg, latest, opts.dryRun)

	githubRepo := os.Getenv("GITHUB_REPOSITORY")

	data := release.BuildData(release.BuildParams{
		Tag:         tag,
		Previous:    latest,
		PreviousTag: prevTag,
		Changes:     ch,
		Headline:    headline,
		KnownIssues: knownIssues,
		Prerelease:  false,
		Commit:      commit,
		Version:     base,
		Root:        root,
		EnrichPR:    !opts.dryRun,
		GithubRepo:  githubRepo,
	})

	entry := release.RenderChangelog(data, githubRepo)

	if opts.dryRun {
		fmt.Fprintf(os.Stderr, "Would create tag: %s\n", tag)
		fmt.Fprintf(os.Stderr, "\nChangelog entry:\n%s\n", entry)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	// Pre-flight: fail before touching the working tree if the tag already
	// exists or no committer identity is available, so a re-run can't half-apply
	// a release and then leave it stranded with "No pending changes".
	if exists, err := git.TagExists(root, tag); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("tag %s already exists; the release may have already been created", tag)
	}
	if !git.IdentityConfigured(root) {
		return fmt.Errorf("no git committer identity configured; set user.name and user.email (or GIT_AUTHOR_*/GIT_COMMITTER_* env) before releasing")
	}
	if err := validateVersionFiles(root, cfg); err != nil {
		return err
	}

	if err := release.WriteJSON(opts.output, data); err != nil {
		return err
	}
	if opts.notes != "" {
		if err := release.WriteNotes(opts.notes, data, notesOptions(cfg, maintainers)); err != nil {
			return err
		}
	}

	// Snapshot HEAD so any failure after this point can roll the working tree,
	// index, and branch ref back to a clean pre-release state.
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

	// Version files go first: Update writes nothing when it fails, so a bad
	// file stops the release before CHANGELOG.md (which reset --hard cannot
	// remove while it is still untracked) is written.
	versioned, err := versionfile.Update(root, cfg.VersionFiles, base)
	if err != nil {
		return rollback(err)
	}

	changelogPath := filepath.Join(root, "CHANGELOG.md")
	existing, err := os.ReadFile(changelogPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading CHANGELOG.md: %w", err)
	}
	previous := release.MarkYanked(string(existing), cfg.TagPrefix, data.Yanks)
	if err := os.WriteFile(changelogPath, []byte(entry+"\n"+previous), 0644); err != nil {
		return fmt.Errorf("writing CHANGELOG.md: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Updated CHANGELOG.md")

	// Archive consumed change files (and the headline) to
	// .changes/released/<version>/ and stage exactly the paths we touched, so
	// unrelated edits under .changes/ are not swept into the release commit. A
	// failed move is fatal: a file left behind would double-count on the next
	// release. Known issues stay until someone removes them.
	staged := append([]string{"CHANGELOG.md"}, versioned...)
	archiveRel := filepath.Join(changes.Dir, changes.ArchiveDir, base)
	if err := os.MkdirAll(filepath.Join(root, archiveRel), 0755); err != nil {
		return rollback(fmt.Errorf("creating %s: %w", archiveRel, err))
	}
	archive := func(name string) (bool, error) {
		from := filepath.Join(changes.Dir, name)
		to := filepath.Join(archiveRel, name)
		if err := os.Rename(filepath.Join(root, from), filepath.Join(root, to)); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("archiving %s: %w", from, err)
		}
		staged = append(staged, from, to)
		return true, nil
	}
	archived := 0
	for _, c := range ch {
		if _, err := archive(c.Filename); err != nil {
			return rollback(err)
		}
		archived++
	}
	if ok, err := archive(changes.HeadlineFile); err != nil {
		return rollback(err)
	} else if ok {
		fmt.Fprintln(os.Stderr, "Archived .changes/_headline.md")
	}
	fmt.Fprintf(os.Stderr, "Archived %d change files to %s\n", archived, archiveRel)

	if err := git.Add(root, staged...); err != nil {
		return rollback(err)
	}
	if err := git.Commit(root, "chore: release "+tag); err != nil {
		return rollback(err)
	}
	if err := git.CreateTag(root, tag); err != nil {
		return rollback(err)
	}
	fmt.Fprintf(os.Stderr, "Created tag: %s\n", tag)
	writeGitHubOutputs(tag, tag, latest, false)
	return nil
}

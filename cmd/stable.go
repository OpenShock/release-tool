package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/OpenShock/release-tool/internal/git"
	"github.com/OpenShock/release-tool/internal/release"
)

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

	latest, err := git.LatestStableTag(root, cfg.TagPrefix)
	if err != nil {
		return err
	}
	base, err := release.ComputeNext(ch, latest)
	if err != nil {
		return err
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
		Headline:    changes.ReadHeadline(root),
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

	if err := release.WriteJSON(opts.output, data); err != nil {
		return err
	}
	if opts.notes != "" {
		if err := release.WriteNotes(opts.notes, data, maintainers); err != nil {
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

	changelogPath := filepath.Join(root, "CHANGELOG.md")
	existing, err := os.ReadFile(changelogPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading CHANGELOG.md: %w", err)
	}
	if err := os.WriteFile(changelogPath, []byte(entry+"\n"+string(existing)), 0644); err != nil {
		return fmt.Errorf("writing CHANGELOG.md: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Updated CHANGELOG.md")

	// Remove consumed change files and stage exactly the paths we touched
	// (CHANGELOG.md plus each removed file) so unrelated edits under .changes/
	// are not swept into the release commit. A failed removal is fatal — a
	// resurrected file would double-count on the next release.
	staged := []string{"CHANGELOG.md"}
	removed := 0
	for _, c := range ch {
		rel := filepath.Join(changes.Dir, c.Filename)
		if err := os.Remove(filepath.Join(root, rel)); err != nil && !os.IsNotExist(err) {
			return rollback(fmt.Errorf("removing %s: %w", rel, err))
		}
		staged = append(staged, rel)
		removed++
	}
	headlineRel := filepath.Join(changes.Dir, changes.HeadlineFile)
	if err := os.Remove(filepath.Join(root, headlineRel)); err == nil {
		staged = append(staged, headlineRel)
		fmt.Fprintln(os.Stderr, "Removed .changes/_headline.md")
	} else if !os.IsNotExist(err) {
		return rollback(fmt.Errorf("removing %s: %w", headlineRel, err))
	}
	fmt.Fprintf(os.Stderr, "Removed %d change files\n", removed)

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

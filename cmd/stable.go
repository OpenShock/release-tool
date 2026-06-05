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

func runRelease() error {
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

	prevTag, maintainers := enrichment(root, cfg, latest)

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
		EnrichPR:    !dryRun,
		GithubRepo: githubRepo,
	})

	entry := release.RenderChangelog(data, githubRepo)

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
		if err := release.WriteNotes(notes, data, maintainers); err != nil {
			return err
		}
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
	writeGitHubOutputs(tag, false)
	return nil
}

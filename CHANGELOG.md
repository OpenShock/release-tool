## [v1.0.1] - 2026-10-29

### Notices

- **IMPORTANT**: **Notice levels are now caution, warning, important and note.** Change files using info or error fail to parse; rename info to note and error to caution.
- **IMPORTANT**: **release.json is now schema version 2.** Consumers should read the new fields and the renamed notice levels.
- **IMPORTANT**: **The PR check now fails without a change file.** Set require-changes to false to keep the old warning, or list paths that never need one under ignore_paths in .changes/config.json.
- **IMPORTANT**: **A prerelease with version_files configured now pushes a commit.** Push HEAD together with the tag, as OpenShock/actions/publish-tag does, so the tag's commit lands on the branch.
- **NOTE**: Unknown frontmatter keys are now errors, and files in .changes/ starting with "_" are reserved.

### Added
- Set the version in project files on release
- Override the computed version of a stable release

### Changed
- **Breaking:** Put safety, breaking changes and upgrade requirements first in the release notes ([#15](https://github.com/OpenShock/release-tool/pull/15))

**Full Changelog: [v0.6.0 -> v1.0.1](https://github.com/OpenShock/release-tool/compare/v0.6.0...v1.0.1)**

## [v0.6.0] - 2026-10-27

### Changed
- Trim, link and order the generated release notes

**Full Changelog: [v0.5.1 -> v0.6.0](https://github.com/OpenShock/release-tool/compare/v0.5.1...v0.6.0)**

## [v0.5.1] - 2026-07-28

### Security
- Bump OpenShock/actions publish-tag pin to v1.1.1 (code-injection fix)

**Full Changelog: [v0.5.0 -> v0.5.1](https://github.com/OpenShock/release-tool/compare/v0.5.0...v0.5.1)**

## [v0.5.0] - 2026-07-28

### Changed
- Fold release-mode git prep (rebase + fetch tags) into the action itself (#12)

### Notices

- **INFO**: Workflows that were manually rebasing/fetching tags before calling release-tool in release mode can remove those now-redundant steps.

**Full Changelog: [v0.4.0 -> v0.5.0](https://github.com/OpenShock/release-tool/compare/v0.4.0...v0.5.0)**

## [v0.3.0] - 2026-07-08

### Changed
- Bump `actions/cache` from 4.3.0 to 6.1.0 in the github-actions dependency group

**Full Changelog: [v0.2.2 -> v0.3.0](https://github.com/OpenShock/release-tool/compare/v0.2.2...v0.3.0)**

## [v0.2.2] - 2026-06-25

### Fixed
- Use a normalized absolute path for the binary cache so the save step works for `uses: ./` self-invocation (github.action_path ends in `/.`, which actions/cache rejected as an invalid pattern, silently skipping the save) (#7)

**Full Changelog: [v0.2.1 -> v0.2.2](https://github.com/OpenShock/release-tool/compare/v0.2.1...v0.2.2)**

## [v0.2.1] - 2026-06-25

### Fixed
- Cache the compiled binary keyed on a source content hash instead of recompiling via `go run .` every invocation; fixes the non-functional setup-go module cache (its cache-dependency-path sat outside GITHUB_WORKSPACE) and removes the per-run compile cost (#6)

**Full Changelog: [v0.2.0 -> v0.2.1](https://github.com/OpenShock/release-tool/compare/v0.2.0...v0.2.1)**

## [v0.2.0] - 2026-06-25

### Added
- Expose the computed version as action outputs: `status` emits `next-version`, `previous-version`, and `bump`; `release`/`prerelease` emit `version` (set even on the no-tag path) (#5)

**Full Changelog: [v0.1.0 -> v0.2.0](https://github.com/OpenShock/release-tool/compare/v0.1.0...v0.2.0)**

## [v0.1.0] - 2026-06-22

### Added
- A single `release` command cuts a stable, prerelease, or no-tag build based on per-branch config in `.changes/config.json`
- Change files in `.changes/` (kind, breaking, mandatory) drive the semver bump and generate a Keep-a-Changelog `CHANGELOG.md`, validated notices, and a structured `release.json`
- Release notes thank commit contributors and link PRs, with per-change override or suppression of the PR number
- GitHub Action with `release`, `status`, and `check` modes for use in release and pull-request workflows
- `init` scaffolds the `.changes/` config and the GitHub Actions release and check workflows
- Pull-request check validates the change files a PR adds and posts a sticky verdict comment, flagging missing, invalid, or `pr:`-pinned files

# Version v0.1.0-alpha.1 Release Notes

- Make CLI interactive [cli]
- Releases are now triggered manually via workflow_dispatch with a mode selector (stable, rc, alpha, beta) [ci]
  Removed the automatic prerelease workflow that ran on every push to master/develop/beta.
  All release cuts (including prereleases) now require an explicit manual dispatch.
- stable command now accepts --prerelease-label to produce a versioned prerelease tag [cli]
  When a prerelease label is provided (e.g. alpha, beta, rc), the stable command creates
  a tag like v1.2.0-alpha.1 instead of v1.2.0, while still updating CHANGELOG.md,
  removing .changes/ files, and committing, behaviour that was previously only available
  for bare stable releases.
- Add tag_prefix to .changes/config.json [config]
  Set "tag_prefix": "v" to produce v-prefixed tags (e.g. v1.2.0). Defaults to no prefix.



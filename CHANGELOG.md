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
  All release cuts — including prereleases — now require an explicit manual dispatch.
- stable command now accepts --prerelease-label to produce a versioned prerelease tag [cli]
  When a prerelease label is provided (e.g. alpha, beta, rc), the stable command creates
  a tag like v1.2.0-alpha.1 instead of v1.2.0, while still updating CHANGELOG.md,
  removing .changes/ files, and committing — behaviour that was previously only available
  for bare stable releases.
- Add tag_prefix to .changes/config.json [config]
  Set "tag_prefix": "v" to produce v-prefixed tags (e.g. v1.2.0). Defaults to no prefix.



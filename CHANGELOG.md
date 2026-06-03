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



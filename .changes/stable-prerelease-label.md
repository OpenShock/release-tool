---
type: minor
categories: [cli]
---
stable command now accepts --prerelease-label to produce a versioned prerelease tag

When a prerelease label is provided (e.g. alpha, beta, rc), the stable command creates
a tag like v1.2.0-alpha.1 instead of v1.2.0, while still updating CHANGELOG.md,
removing .changes/ files, and committing — behaviour that was previously only available
for bare stable releases.

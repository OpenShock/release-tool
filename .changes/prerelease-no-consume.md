---
kind: changed
---
Prereleases no longer consume change files or update the changelog

## Release Note
The rc command now creates a lightweight tag from pending changes without touching .changes/ files or CHANGELOG.md. Only stable releases consume changes and write the changelog.

---
kind: changed
---
Overhaul change file format: kind, mandatory, safety, chore, KaC changelog

## Release Note
The change file format now uses a semantic kind field (added, changed, deprecated, removed, fixed, security, safety, chore) instead of the old type field (major, minor, patch). Semver is derived automatically from the kind and an optional breaking flag.
A new mandatory flag marks versions that must be installed before skipping ahead.
CHANGELOG.md output now follows Keep a Changelog format with grouped sections per kind.
release.json uses plain strings throughout — no markdown formatting.

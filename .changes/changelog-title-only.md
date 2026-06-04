---
type: minor
categories: [cli]
---

Changelog entries show title only; release note gains title and body

`CHANGELOG.md` now renders a single bullet per change (title line only).
The change body is preserved in `release.json` for API consumers.
The `## Release Note` section now supports a title line and optional detail
body, both stored as structured fields in `release.json`.

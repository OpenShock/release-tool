---
type: minor
categories: [cli]
---
`init` scaffolds config.json and GitHub Actions workflows

## Release Note
`init` now creates `.changes/config.json` (with branch config derived from `--branches`) and
both two-stage PR check workflows (`.github/workflows/check-changes.yml` and
`pr-check-comment.yml`). Pass `--action-ref` to pin the action to a SHA. Use `--no-workflows`
to skip workflow generation.

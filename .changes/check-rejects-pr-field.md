---
kind: changed
---
PR check rejects change files with explicit `pr:` frontmatter

## Release Note
Change files submitted in a PR must not set `pr:` to a number or `null` — the
PR number is assigned automatically from git history at release time. Setting it
manually in a PR could link the change to the wrong PR or suppress the link
entirely. The `check` command now reports these as `invalid`.

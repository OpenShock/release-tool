---
type: minor
categories: [cli]
---
Change files can pin or suppress the PR number via frontmatter

## Release Note
The `pr:` field is tri-state: omit it to derive the PR from git history (the existing behavior), set an integer to use it verbatim, or set `pr: null` to suppress the PR link entirely.

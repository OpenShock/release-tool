# release-tool

Manages releases for OpenShock projects. Drop change files in `.changes/`, run the tool, get a semver tag and `release.json`.

## How it works

Each pending change lives as a small Markdown file in `.changes/`:

```
---
type: minor
---
Add support for X
```

Run `release-tool stable` (or use the GitHub Action) and it:
- Computes the next semver based on the highest bump type across all pending changes
- Writes `release.json` with structured change data
- Updates `CHANGELOG.md`
- Commits, tags, and cleans up the `.changes/` files

## Creating change files

```sh
release-tool new
```

Walks you through it interactively. In CI, pass flags directly:

```sh
release-tool new "Add support for X" --type minor
```

## GitHub Action

```yaml
- uses: OpenShock/release-tool@v1
  with:
    mode: stable          # or rc
    prerelease-label: rc  # rc | alpha | beta | ...
    git-sha: false        # append +g<sha> to prerelease tags
    notes-output: release-notes.md
```

Outputs: `tag`, `prerelease`, `skip`.

## Change file format

```markdown
---
type: minor          # major | minor | patch
breaking: false      # optional; major defaults to true
categories: [api]    # optional
---
Title shown in changelog

Optional extended body.

## Release Note
Plain-language note for end users (goes into release.json, not the changelog).

## Notices
- warning: something users must know before upgrading
```

# release-tool

`release-tool` manages releases from change files in `.changes/`. It computes the next semver, writes `release.json`, can generate markdown release notes, and creates the corresponding git tag.

## Install

### Fastest: download a release binary

Pick the binary for your platform from [GitHub Releases](https://github.com/OpenShock/release-tool/releases).

Common installs:

#### Linux x86_64

```sh
curl -Lo /usr/local/bin/release-tool \
  https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-linux-amd64
chmod +x /usr/local/bin/release-tool
```

#### macOS Apple Silicon

```sh
curl -Lo /usr/local/bin/release-tool \
  https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-darwin-arm64
chmod +x /usr/local/bin/release-tool
```

#### macOS Intel

```sh
curl -Lo /usr/local/bin/release-tool \
  https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-darwin-amd64
chmod +x /usr/local/bin/release-tool
```

#### Windows PowerShell

```powershell
Invoke-WebRequest `
  -Uri https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-windows-amd64.exe `
  -OutFile $env:USERPROFILE\bin\release-tool.exe
```

If you use a different architecture, download the matching asset from Releases.

### Alternative: install with Go

```sh
go install github.com/OpenShock/release-tool@latest
```

If `release-tool` is not found afterwards, add your Go bin directory to `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

### Check it works

```sh
release-tool --help
```

## Workflow

1. Initialise a repo once:

```sh
release-tool init
```

2. Add change files as work lands:

```sh
release-tool new
```

Or non-interactively:

```sh
release-tool new "Add support for X" --type minor --categories api,cli
```

3. Inspect pending changes:

```sh
release-tool status
```

4. Cut a release:

```sh
release-tool stable
```

`stable`:
- Computes the next version from pending `.changes/*.md`
- Writes `release.json`
- Optionally writes markdown notes via `--notes`
- Prepends a new entry to `CHANGELOG.md`
- Removes consumed change files
- Commits the changelog/update and creates a stable tag

For prereleases, use:

```sh
release-tool --prerelease-label beta rc
release-tool --prerelease-label develop --git-sha rc --allow-empty
```

`rc` does **not** consume `.changes/` files or update `CHANGELOG.md`; it only writes release data and creates a prerelease tag.

## Change file format

```markdown
---
type: minor          # major | minor | patch
breaking: false      # optional; major defaults to true
categories: [api]    # optional
---
Title shown in changelog

Optional extended body shown in the changelog.

## Release Note
Plain-language note for end users. Included in `release.json`, not in `CHANGELOG.md`.

## Notices
- warning: something users must know before upgrading
- info: optional migration or rollout note
```

Special files in `.changes/`:
- `README.md`: local format reference created by `release-tool init`
- `_headline.md`: optional markdown shown at the top of the generated changelog entry
- `config.json`: optional repo config

Example `.changes/config.json`:

```json
{
  "tag_prefix": "v"
}
```

## Common flags

Global flags available to `stable`, `rc`, `status`, `init`, and `new`:

- `--dry-run`: preview without writing files, committing, or tagging
- `--output <path>`: where to write `release.json` (default `release.json`)
- `--notes <path>`: write markdown release notes
- `--prerelease-label <label>`: prerelease label such as `alpha`, `beta`, `rc`, or `develop`
- `--git-sha`: append `+g<sha>` build metadata to prerelease tags
- `--root <path>`: operate on another repository root

## GitHub Action

The composite action wraps the CLI and exposes three modes:

```yaml
- uses: OpenShock/release-tool@v1
  with:
    mode: stable               # stable | beta | develop
    dry-run: false
    output: release.json
    notes-output: release-notes.md
    prerelease-label: beta     # optional override for beta/develop
    git-sha: false             # append +g<sha> to prerelease tags
```

Mode behavior:
- `stable`: consumes pending change files and updates `CHANGELOG.md`
- `beta`: creates prerelease tags from changes since the last beta or stable tag
- `develop`: creates prerelease tags from changes since the last develop, beta, or stable tag, always with git SHA metadata and allowing empty cuts

Action outputs:
- `tag`: created tag, empty when skipped
- `prerelease`: `true` for prerelease tags
- `skip`: `true` when no release was created

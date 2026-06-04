# release-tool

`release-tool` automates semver releases driven by change files in `.changes/`. It reads per-branch config to decide whether a push should produce a stable release, a prerelease tag, or a no-tag build artifact — all from one `release` command that CI calls the same way on every branch.

## Install

### Fastest: download a release binary

Pick the binary for your platform from [GitHub Releases](https://github.com/OpenShock/release-tool/releases).

```sh
# Linux x86_64
curl -Lo /usr/local/bin/release-tool \
  https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-linux-amd64
chmod +x /usr/local/bin/release-tool

# macOS Apple Silicon
curl -Lo /usr/local/bin/release-tool \
  https://github.com/OpenShock/release-tool/releases/latest/download/release-tool-darwin-arm64
chmod +x /usr/local/bin/release-tool
```

### Alternative: install with Go

```sh
go install github.com/OpenShock/release-tool@latest
```

## Quick start

```sh
# Bootstrap a new repo (creates .changes/ and .github/workflows/)
release-tool init --branches master,beta --develop develop \
  --action-ref "OpenShock/release-tool@v1.0.0" --tag-prefix v

# Create a change file for work you're about to commit
release-tool new "Add user authentication" --type minor --categories api

# Check pending changes and next version
release-tool status

# Manually cut a release (CI handles this automatically)
release-tool release
```

## How it works

Everything is driven by `.changes/config.json`:

```json
{
  "tag_prefix": "v",
  "categories": ["api", "frontend", "ci"],
  "branches": {
    "master":  { "release": "stable" },
    "beta":    { "release": "prerelease", "label": "beta" },
    "develop": { "release": "none", "label": "develop", "sha": true }
  }
}
```

When a push lands on a configured branch, `release-tool release` reads the branch config and:

| `release` value | Behaviour |
|---|---|
| `stable` | Computes next semver, writes `CHANGELOG.md`, removes change files, commits, creates tag |
| `prerelease` | Computes next semver + label (e.g. `1.2.0-beta.3`), writes release data, creates tag — change files stay |
| `none` | Computes version with SHA metadata (e.g. `1.2.0-develop+gabc123`), writes release data — **no git tag, no changelog** |

If there are no change files since the last stable tag, the tool exits with `skip=true` and does nothing.

## Branch model

```
feature/* → develop  (none:  release.json + SHA version, no tag)
develop   → beta     (prerelease: release.json + beta.N tag)
beta      → master   (stable: release.json + vX.Y.Z tag + CHANGELOG.md)
```

Change files are never consumed by prerelease or none branches. Only a stable release removes them. This means:
- A develop build always reflects all unreleased changes
- After a stable release, develop sees only changes added since that tag (git history-based, not filesystem-based)

## Setting up CI

### Automated releases

Add a `release.yml` workflow that triggers on push to your release branches:

```yaml
on:
  push:
    branches: [master, beta, develop]
    paths-ignore:
      - 'CHANGELOG.md'
      - '.changes/**'

name: release

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    if: github.actor != 'github-actions[bot]'
    steps:
      - uses: actions/checkout@<sha> # v6
        with:
          fetch-depth: 0
          token: ${{ secrets.RELEASE_TOKEN }}   # PAT required — see below

      - name: Configure git
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"

      - name: Rebase on top of latest remote
        run: git pull --rebase --autostash origin "${GITHUB_REF_NAME}"

      - name: Fetch all tags
        run: git fetch --tags

      - uses: OpenShock/release-tool@v1
        id: meta
        with:
          mode: release
          notes-output: release-notes.md
        env:
          GITHUB_TOKEN: ${{ secrets.RELEASE_TOKEN }}

      - name: Push commit and tag
        if: steps.meta.outputs.tag != ''
        run: git push origin HEAD "${{ steps.meta.outputs.tag }}"

      - name: Create GitHub release
        if: steps.meta.outputs.tag != ''
        env:
          GH_TOKEN: ${{ secrets.RELEASE_TOKEN }}
        run: |
          ARGS=("${{ steps.meta.outputs.tag }}" release.json --title "${{ steps.meta.outputs.tag }}" --notes-file release-notes.md)
          [ "${{ steps.meta.outputs.prerelease }}" = "true" ] && ARGS+=(--prerelease)
          gh release create "${ARGS[@]}"
```

> **`RELEASE_TOKEN`** must be a Personal Access Token (PAT) with `repo` scope. The default `GITHUB_TOKEN` cannot push tags in a way that triggers other workflows (e.g. `ci-build`). Create the PAT, add it as a repository secret named `RELEASE_TOKEN`.

### PR change-file check

Run `release-tool init` to generate both workflow files automatically, or add them manually:

**`.github/workflows/check-changes.yml`** — runs on `pull_request` with read-only permissions (fork-safe):

```yaml
on:
  pull_request:
    branches: [master, beta, develop]
    types: [opened, reopened, synchronize, ready_for_review, labeled, unlabeled]

name: check-changes

permissions:
  contents: read

jobs:
  check:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    if: >-
      !github.event.pull_request.draft &&
      !contains(github.event.pull_request.labels.*.name, 'no-changelog')
    steps:
      - uses: actions/checkout@<sha> # v6
        with:
          fetch-depth: 0

      - uses: OpenShock/release-tool@v1
        with:
          mode: check
          base-ref: ${{ github.event.pull_request.base.ref }}
          base-sha: ${{ github.event.pull_request.base.sha }}
          pr-number: ${{ github.event.pull_request.number }}

      - uses: actions/upload-artifact@<sha> # v7
        if: always()
        with:
          name: release-check
          path: release-check.json
          if-no-files-found: warn
```

**`.github/workflows/pr-check-comment.yml`** — runs on `workflow_run` with write permissions, never executes fork code:

```yaml
on:
  workflow_run:
    workflows: [check-changes]
    types: [completed]

name: pr-check-comment

permissions:
  pull-requests: write

jobs:
  comment:
    runs-on: ubuntu-latest
    if: github.event.workflow_run.event == 'pull_request'
    steps:
      - name: Download verdict
        id: download
        continue-on-error: true
        uses: actions/download-artifact@<sha> # v8
        with:
          name: release-check
          run-id: ${{ github.event.workflow_run.id }}
          github-token: ${{ secrets.GITHUB_TOKEN }}

      - name: Post, update, or remove sticky comment
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_REPO: ${{ github.repository }}
          DOWNLOAD_OK: ${{ steps.download.outcome == 'success' }}
          FALLBACK_PR: ${{ github.event.workflow_run.pull_requests[0].number }}
        run: |
          if [ "$DOWNLOAD_OK" = "true" ]; then
            STATE=$(jq -r '.state' release-check.json)
            PR=$(jq -r '.pr' release-check.json)
            BODY=$(jq -r '.body' release-check.json)
          else
            STATE="skip"; PR="$FALLBACK_PR"; BODY=""
          fi
          [ -z "$PR" ] || [ "$PR" = "0" ] || [ "$PR" = "null" ] && exit 0
          EXISTING=$(gh api "repos/$GH_REPO/issues/$PR/comments" \
            --jq '[.[] | select(.body | contains("<!-- release-tool-check -->"))] | first | .id // empty')
          if [ "$STATE" = "skip" ]; then
            [ -n "$EXISTING" ] && gh api --method DELETE "repos/$GH_REPO/issues/comments/$EXISTING"
          elif [ -n "$EXISTING" ]; then
            gh api --method PATCH "repos/$GH_REPO/issues/comments/$EXISTING" --field body="$BODY"
          else
            gh api --method POST "repos/$GH_REPO/issues/$PR/comments" --field body="$BODY"
          fi
```

The two-stage split is the key security property: fork code runs in stage 1 (read-only token, no secrets), the write token only touches stage 2 (base branch code only, no fork code).

Add the `no-changelog` label to a PR to skip the check for intentional non-release changes (dependency bumps, CI tweaks, docs).

## Change file format

```markdown
---
type: minor          # major | minor | patch  (required)
breaking: false      # optional; defaults to true when type is major
categories: [api]    # optional; validated against allowlist if set in config.json
---
Title shown in changelog (required, first line)

Optional extended body shown in the changelog entry.

## Release Note
Plain-language note for end users. Included in release.json, not in CHANGELOG.md.

## Notices
- warning: something users must know before upgrading
- info: optional migration step
- error: something that will break
```

**`pr` field** — do not set this in a PR. It is assigned automatically at release time from git history. Setting it (to a number or `null`) will cause the PR check to fail.

At release time, `pr` is tri-state:
- **absent**: derived from git log
- **integer** (`pr: 123`): used verbatim
- **`pr: null`**: PR link suppressed

Notice levels must be `info`, `warning`, or `error`.

## CLI reference

```
release-tool [command]

Commands:
  init        Bootstrap .changes/ and GitHub Actions workflows
  new         Create a change file interactively or from flags
  status      Show pending changes and next version (no side effects)
  release     Run stable/prerelease/none based on branch config (used by CI)
  prerelease  Create a prerelease tag directly (manual override)
  check       Validate change files added by a PR, write verdict JSON
```

### `init`

```sh
release-tool init \
  --branches master,beta \       # stable first, then prerelease
  --develop develop \            # release:none + sha:true branches
  --categories api,ci,frontend \ # allowlist (empty = any)
  --tag-prefix v \
  --action-ref "OpenShock/release-tool@<sha>" \
  --no-workflows                 # skip .github/workflows/ generation
```

Idempotent — skips files that already exist.

### `new`

```sh
release-tool new "Fix crash on boot" --type patch --categories firmware
```

Interactive if title is omitted.

### `status`

```sh
release-tool status
```

Prints pending change files and the next version that would be created. No files written, no tags created.

### `release`

```sh
release-tool release [--dry-run] [--output release.json] [--notes release-notes.md]
```

Reads the current branch from config and dispatches:
- `stable` → consume changes, write changelog, create tag
- `prerelease` → create tag, leave change files
- `none` → write release data only, no tag

### `prerelease`

```sh
release-tool prerelease [--prerelease-label rc] [--git-sha] [--dry-run]
```

Direct prerelease invocation for manual use. Reads the current branch for context but overrides label/sha from flags. Always creates a tag (unlike `release` on a `none` branch).

### `check`

```sh
release-tool check --base master --against <base-sha> --pr <pr-number> --out release-check.json
```

Writes a verdict JSON used by the PR comment stage. Exit code is non-zero on `invalid`.

## Action reference

```yaml
- uses: OpenShock/release-tool@v1
  with:
    mode: release   # release | status | check
    # release mode:
    output: release.json
    notes-output: release-notes.md
    dry-run: false
    # check mode:
    base-ref: ${{ github.event.pull_request.base.ref }}
    base-sha: ${{ github.event.pull_request.base.sha }}
    pr-number: ${{ github.event.pull_request.number }}
    # advanced:
    go-version: '1.25'
```

Outputs: `tag`, `prerelease`, `skip`.

## Contributors & PR enrichment

When `GITHUB_TOKEN` is set with sufficient permissions, the tool enriches releases with GitHub data:

- **PR numbers** are derived for change files that don't pin one
- **Contributors** — commit authors since the previous tag are listed in `release.json` and in a `### Contributors` section in the release notes, excluding maintainers (admin/maintain collaborators) and bot accounts

Both require `fetch-depth: 0` so the previous tag is reachable. Enrichment is skipped under `--dry-run`.

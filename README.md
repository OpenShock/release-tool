# release-tool

`release-tool` automates semver releases driven by change files in `.changes/`. It reads per-branch config to decide whether a push should produce a stable release, a prerelease tag, or a no-tag build artifact, all from one `release` command that CI calls the same way on every branch.

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
release-tool new "Add user authentication" --kind added

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
| `stable` | Computes next semver, writes `CHANGELOG.md` and the `version_files`, moves consumed change files to `.changes/released/<version>/`, commits, creates tag |
| `prerelease` | Computes next semver + label (e.g. `1.2.0-beta.3`), writes release data, commits the `version_files`, creates tag; change files stay |
| `none` | Computes version with SHA metadata (e.g. `1.2.0-develop+gabc123`), writes release data, **no git tag, no changelog** |

If there are no change files since the last stable tag, the tool exits with `skip=true` and does nothing.

## Branch model

```
feature/* → develop  (none:  release.json + SHA version, no tag)
develop   → beta     (prerelease: release.json + beta.N tag)
beta      → master   (stable: release.json + vX.Y.Z tag + CHANGELOG.md)
```

Change files are never consumed by prerelease or none branches. Only a stable release moves them into `.changes/released/<version>/`. This means:
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
          token: ${{ secrets.RELEASE_TOKEN }}   # PAT required, see below

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

**`.github/workflows/check-changes.yml`**: runs on `pull_request` with read-only permissions (fork-safe):

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

**`.github/workflows/pr-check-comment.yml`**: runs on `workflow_run` with write permissions, never executes fork code:

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

Add the `no-changelog` label to a PR to skip the check for intentional non-release changes (dependency bumps, CI tweaks, docs), or list paths that never need a change file in `config.json`:

```json
{
  "ignore_paths": ["docs/**", ".github/**", "**/*.md"],
  "prerelease_notice": "This is a beta. Only install it if you can reflash over USB. The latest stable release is {stable}.",
  "fold_after": 10
}
```

## Version files

List files that record the project version under `version_files`, and every release sets them to the released version (without `tag_prefix`) in its release commit:

```json
{
  "version_files": [{ "path": "frontend/package.json", "json_key": "version" }]
}
```

`json_key` names a top-level string field of a JSON object. Only that value is rewritten, so the file's formatting stays as it was. A stable release puts the files in its `chore: release <tag>` commit. A prerelease, which otherwise makes no commit, commits them as `chore: release <tag>` and tags that commit; push it together with the tag (`OpenShock/actions/publish-tag` pushes `HEAD` and the tag). SHA builds (`--git-sha`) and `none` branches never touch them. A bad entry fails the release before anything is written.

## Change file format

See [`.changes/README.md`](.changes/README.md) (also written by `release-tool init`) for the full format. In short:

```markdown
---
kind: fixed          # safety | security | added | changed | deprecated | removed | fixed | chore
breaking: false      # optional; true forces a major bump
mandatory: false     # optional; install this version before any later one
upgrade:             # optional; rendered as "before you update" alerts
  min_version: 1.4.0
  usb_only: false
  config_reset: false
  no_downgrade: false
advisories: []       # security only: CVE-... or GHSA-... ids
authors: []          # extra GitHub logins to credit
yanks: []            # earlier versions this release withdraws
---
Fix the crash when pairing a shocker

## Release Note
User-facing title line
Plain-language description for end users.

## Notices
- caution: risk of harm or bricking
- warning: behaviour that changes under the user
- important: something the user has to do
- note: good to know
```

Unknown frontmatter keys are errors, so a typo can't silently drop a field.

**`pr` field**: do not set this in a PR. It is assigned automatically at release time from git history. Setting it (to a number or `null`) will cause the PR check to fail.

At release time, `pr` is tri-state:
- **absent**: derived, in order, from a trailing `(#123)` in the title (which is then stripped so it isn't shown twice), the subject of the commit that added the change file (`... (#123)` or `Merge pull request #123 ...`, works offline and in `--dry-run`), and finally the GitHub API
- **integer** (`pr: 123`): used verbatim
- **`pr: null`**: PR link suppressed

Special files start with `_`: `_headline.md` (consumed by the stable release) and `_known-issues.md` (one `- ` item per issue, shown in every release until removed).

### Lint

`check`, `release` and `prerelease` lint change files; `status` reports the findings without failing.

| Severity | Rule |
|---|---|
| error | A breaking change has no `## Release Note` and no notice telling users what to do |
| warning | A title starts in the past tense ("Fixed ...") instead of the imperative ("Fix ...") |
| warning | A safety change has no `## Release Note` |
| warning | A stable release has no `_headline.md` |

### How the release body is rendered

The GitHub Release body puts what a reader needs before installing first:

1. **Prerelease warning** (prereleases only), configurable with `prerelease_notice`.
2. **Headline** from `_headline.md`.
3. **Alerts**, one GitHub alert per level (`[!CAUTION]`, `[!WARNING]`, `[!IMPORTANT]`, `[!NOTE]`): a caution when the release has safety changes, withdrawn (`yanks`) versions and every breaking change as warnings, `mandatory` and `upgrade` requirements as important, then each change's notices at their level.
4. **New since `<previous prerelease>`** (numbered prereleases only): the changes added after the previous prerelease of the same cycle. The rest of the body stays cumulative since the last stable release.
5. **Kind sections** in the order Safety, Security, Added, Changed, Deprecated, Removed, Fixed. Breaking entries come first with a `**Breaking:**` prefix, then entries in the order their change files were committed. Entries with the same kind and title are merged into one line listing every PR. List items in a `## Release Note` description go into a collapsed "Details" block. A section with more than `fold_after` (default 10) entries folds its non-breaking entries; Safety and Security never fold.
6. **Known issues** from `_known-issues.md`.
7. **Contributors**, excluding maintainers and bots, with first-time contributors welcomed.
8. **Full Changelog** compare link.

Chores never appear in the GitHub Release body; `CHANGELOG.md` lists them last under "Internal". `CHANGELOG.md` shows the same alerts as a "Notices" list under the version heading. When the repository is known (`GITHUB_REPOSITORY`), PR references and bare `#123` mentions become links, and `<` outside code spans is escaped so titles can't open HTML tags.

### release.json

`release.json` (schema version 2) is the structured record of a release, meant for update UIs and the repository server: `changes[]` (with `audience: user|internal`, `advisories`, `authors`), `upgrade`, `yanks`, `since_prerelease`, `known_issues`, `contributors` and `first_time_contributors`. Notice levels are the four alert types above.

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
  --tag-prefix v \
  --action-ref "OpenShock/release-tool@<sha>" \
  --no-workflows                 # skip .github/workflows/ generation
```

Idempotent: skips files that already exist.

### `new`

```sh
release-tool new "Fix crash on boot" --kind fixed
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

Writes a verdict JSON used by the PR comment stage. Exit code is non-zero on `invalid`, and on `missing` with `--require-changes` (the action's default). Findings are printed as GitHub annotations on the offending file. For a valid PR the comment also states its version impact: the bump level of its change files and the next release version with and without the PR (also in the verdict JSON as `impact`). A PR whose files all match `ignore_paths` in `.changes/config.json` is skipped.

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
    require-changes: true   # fail PRs without a change file
    # advanced:
    go-version: '1.25'
```

Outputs: `tag`, `prerelease`, `skip`.

## Contributors & PR enrichment

When `GITHUB_TOKEN` is set with sufficient permissions, the tool enriches releases with GitHub data:

- **PR numbers** are derived for change files that don't pin one
- **Contributors**: commit authors since the previous tag are listed in `release.json` and in a `### Contributors` section in the release notes, excluding maintainers (admin/maintain collaborators) and bot accounts

Both require `fetch-depth: 0` so the previous tag is reachable. Enrichment is skipped under `--dry-run`.

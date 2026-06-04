---
type: minor
categories: [ci]
---
Two-stage workflow_run PR comment for change file check

## Release Note
A fork-safe pull request comment workflow posts the change-file verdict as a sticky comment. Stage one (`pr-check.yml`) runs on `pull_request` with read-only permissions, executes the `check` command, and uploads the verdict JSON as an artifact. Stage two (`pr-check-comment.yml`) fires on `workflow_run` completion with `pull-requests: write`, downloads the artifact, and posts or updates the `<!-- release-tool-check -->` sticky comment on the PR.

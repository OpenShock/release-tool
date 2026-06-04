---
type: minor
categories: [ci]
---
Add a check command and branches config for pull request validation

The new `check` command (and `mode: check` on the action) validates the change files a pull request adds and writes a verdict of ok, missing, or invalid. Release branches are declared in a `branches` map in `.changes/config.json`; a pull request whose base is not a release branch is skipped. The verdict is designed for a fork-safe two-stage workflow_run setup that posts the result as a sticky pull request comment.

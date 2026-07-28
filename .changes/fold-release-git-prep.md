---
kind: changed
breaking: false
mandatory: false
---
Fold release-mode git prep (rebase + fetch tags) into the action itself

## Release Note
Release mode now rebases onto the latest remote ref and fetches all tags on its own
Previously, workflows had to rebase and fetch tags themselves before invoking release-tool in release mode. The action now does this internally, so the tag always lands on the true tip and version computation always has full tag history, regardless of whether the caller remembered to do it.

## Notices
- info: Workflows that were manually rebasing/fetching tags before calling release-tool in release mode can remove those now-redundant steps.

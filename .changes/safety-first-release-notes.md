---
kind: changed
breaking: true
---
Put safety, breaking changes and upgrade requirements first in the release notes

## Release Note
Release notes open with what a user must know before installing
The GitHub Release body now starts with the prerelease warning, the headline and GitHub alerts for safety changes, breaking changes, upgrade requirements and notices, then lists what is new since the previous prerelease. Safety and Security are the first sections. Known issues, first-time contributors and a Full Changelog link were added, long sections fold, and nothing in a change's release note is dropped anymore. Change files gain upgrade, advisories, authors and yanks fields, and are linted for breaking changes that don't tell users what to do. A stable release archives its change files instead of deleting them.

## Notices
- important: **Notice levels are now caution, warning, important and note.** Change files using info or error fail to parse; rename info to note and error to caution.
- important: **release.json is now schema version 2.** Consumers should read the new fields and the renamed notice levels.
- important: **The PR check now fails without a change file.** Set require-changes to false to keep the old warning, or list paths that never need one under ignore_paths in .changes/config.json.
- note: Unknown frontmatter keys are now errors, and files in .changes/ starting with "_" are reserved.

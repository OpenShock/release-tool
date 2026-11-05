---
kind: changed
---
Credit the pull request that created a change file, not the one that last moved it

## Release Note
Release notes credit the pull request that wrote each change file, even after it was renamed or moved
The PR link was taken from the most recent commit that added the file under its current name, so renaming change files while tidying them credited the tidy-up instead, and the `pr` field had to be set by hand. Renames are now followed back to the commit that created the file. Copies are not followed, so a change file is never credited to an unrelated one with the same text.

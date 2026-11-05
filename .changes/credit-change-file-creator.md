---
kind: changed
---
Credit the pull request that created a change file, not the one that last moved it

## Release Note
Release notes credit the pull request that wrote each change file, even after it was renamed or moved
The PR link was taken from the most recent commit that added the file under its current name, so renaming change files while tidying them credited the tidy-up instead, and the `pr` field had to be set by hand. Renames are now followed back to the commit that created the file. Copies are not followed, so a new change file that only shares text with an existing one keeps its own credit. A commit that deletes one change file and adds another with near-identical text still reads as a rename, since git cannot tell the two apart.

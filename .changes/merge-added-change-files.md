---
kind: fixed
---
Count change files that a merge commit adds

## Release Note
The change file check now finds change files that arrive through a merge commit
A pull request whose change file was added while merging the base branch back in, such as during a conflict resolution, was reported as having no change file. Change files are now found by comparing the pull request against its base, so a file added and later removed no longer counts either.

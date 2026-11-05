---
kind: added
---
Override the computed version of a stable release

## Release Note
Stable releases can skip ahead to a chosen version
Pass `--version` (action input `version`) to release a specific MAJOR.MINOR.PATCH instead of the computed one, for example to skip a tag name GitHub no longer accepts because it belonged to a deleted immutable release. It may skip ahead but never below what the pending changes require, and only applies to stable branches.

---
kind: added
---
Expose the computed version as action outputs: `status` emits `next-version`, `previous-version`, and `bump`; `release`/`prerelease` emit `version` (set even on the no-tag path)

## Release Note
Version available as action outputs
`status` mode now writes machine-readable step outputs (`next-version`, `previous-version`, `bump`) and `release`/`prerelease` expose a `version` output, so consuming workflows can read the version directly instead of parsing logs or re-deriving it from git tags.

---
type: minor
categories: [cli, ci]
---

Branch config now controls release behaviour via a `release` enum

`BranchConfig` gains `release` (`stable` | `prerelease` | `none`), `label`, and `sha` fields.
The old `Prerelease bool` is gone. `release: none` writes `release.json` without creating a
git tag, enabling SHA-versioned develop builds (`1.6.0-develop+gabc123`) that publish to an
API or artifact store without polluting the tag namespace.

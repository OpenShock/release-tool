---
type: minor
categories: [ci]
---
Releases are now triggered manually via workflow_dispatch with a mode selector (stable, rc, alpha, beta)

Removed the automatic prerelease workflow that ran on every push to master/develop/beta.
All release cuts — including prereleases — now require an explicit manual dispatch.

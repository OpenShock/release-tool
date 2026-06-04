---
type: minor
categories: [cli]
---
`release` command auto-dispatches based on branch config

## Release Note
The new `release` command reads `.changes/config.json`, resolves the current branch, and
dispatches to stable or prerelease logic automatically. Manual `rc` and `stable` subcommands
are removed; `rc` is replaced by `prerelease` for direct invocation. CI only ever needs to
call `release`.

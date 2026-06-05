---
kind: fixed
---
Fix subset reads skipping missing files and guard against path escapes

## Release Note
Prerelease change gathering now correctly skips files that no longer exist instead of erroring, and subset file names are reduced to their basename so they cannot resolve outside `.changes/`.

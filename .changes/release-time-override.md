---
kind: added
---
Record a given release time via SOURCE_DATE_EPOCH

## Release Note
Releases can record a given release time
Set `SOURCE_DATE_EPOCH` (Unix seconds) to record that time as `released_at` and as the CHANGELOG.md date instead of now, so a release can be replayed and reproduce its commit exactly.

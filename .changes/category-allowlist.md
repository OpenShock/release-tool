---
kind: added
---
Optional category allowlist in config.json

## Release Note
Set `categories` in `.changes/config.json` to restrict which category labels change files may use. Files declaring an unknown category fail validation. When the list is omitted, any category is accepted as before.

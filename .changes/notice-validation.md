---
type: minor
categories: [cli]
---
Notice lines are now validated

## Release Note
Notice levels must be `info`, `warning`, or `error`, and each line must follow the `- level: message` form. Invalid levels and malformed lines now fail validation instead of being silently dropped.

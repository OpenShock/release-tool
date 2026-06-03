# Change Files

Each `.md` file in this directory describes one pending change that will be
included in the next release.

## Format

```
---
type: minor        # major | minor | patch
breaking: false    # optional; major defaults to true
categories: []     # optional list of labels
---
Title of the change (first line, required)

Optional longer description in Markdown.

## Summary
Optional short summary included in release.json but not the changelog.

## Notices
- warning: something users must know before upgrading
- info: optional note or migration step
```

## File naming

Name the file after the change (e.g. `add-user-auth.md`).
Run `release-tool new "<title>" --type minor` to generate one automatically.

## Special files

- `_headline.md` - optional release headline shown at the top of the changelog entry

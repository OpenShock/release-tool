---
type: minor
categories: [ci]
---
Action gains a status mode

The composite action now accepts `mode: status`, which validates pending change files and prints the next version without creating a tag. Useful as a pull request check.

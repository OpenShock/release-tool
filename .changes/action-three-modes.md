---
type: minor
categories: [ci]
---

Action pared down to three modes: `release`, `status`, `check`

`prerelease-label` and `git-sha` inputs are removed from the action; those values now live in
`.changes/config.json` branch config. CLI flags (`--dry-run`, `--output`, `--notes`,
`--prerelease-label`, `--git-sha`) are scoped to the commands that use them instead of being
global, so `status` and `check` show a clean help output.

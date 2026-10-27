---
kind: changed
breaking: false
mandatory: false
---
Trim, link and order the generated release notes

## Release Note
Release notes are shorter, link their PRs and list changes in the order they landed
List items in a change's release note are left out of the GitHub Release body, which keeps implementation detail out of it; release.json still carries them. A "(#123)" written into a title is used as the PR instead of being rendered twice, and the PR is also read offline from the subject of the commit that added the change file, so dry runs show it too. PR references and bare #123 mentions become links in the release body and CHANGELOG when the repository is known. Within each section, breaking changes come first and the rest follow the order their change files were committed instead of the filename order.

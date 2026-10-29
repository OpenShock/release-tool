---
kind: added
---
Set the version in project files on release

## Release Note
Releases can update version numbers in project files
List files such as package.json under `version_files` in .changes/config.json, each with the `json_key` that holds the version. Stable releases and prereleases set it to the released version in their release commit, so the files always match the tag. A prerelease now makes that commit before tagging when version files are configured.

## Notices
- important: **A prerelease with version_files configured now pushes a commit.** Push HEAD together with the tag, as OpenShock/actions/publish-tag does, so the tag's commit lands on the branch.

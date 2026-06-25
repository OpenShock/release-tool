---
kind: fixed
---
Use a normalized absolute path for the binary cache so the save step works for `uses: ./` self-invocation (github.action_path ends in `/.`, which actions/cache rejected as an invalid pattern, silently skipping the save)

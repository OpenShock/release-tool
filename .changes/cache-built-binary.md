---
kind: fixed
---
Cache the compiled binary keyed on a source content hash instead of recompiling via `go run .` every invocation; fixes the non-functional setup-go module cache (its cache-dependency-path sat outside GITHUB_WORKSPACE) and removes the per-run compile cost

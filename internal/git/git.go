package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ghAPI runs `gh api <args...>` in root and returns stdout. The gh CLI handles
// auth (GH_TOKEN); a non-nil error means gh is missing, unauthenticated, or the
// request failed, and carries gh's own stderr for diagnostics.
func ghAPI(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", append([]string{"api"}, args...)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh api: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gh api: %w", err)
	}
	return out, nil
}

func run(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func LatestStableTag(root, prefix string) (string, error) {
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `(\d+\.\d+\.\d+)$`)
	tags, err := run(root, "tag", "--sort=-v:refname")
	if err != nil {
		return "", err
	}
	for _, tag := range strings.Split(tags, "\n") {
		tag = strings.TrimSpace(tag)
		if m := re.FindStringSubmatch(tag); m != nil {
			return m[1], nil
		}
	}
	return "", nil
}

// LatestPrereleaseNumber returns the highest N seen in tags of the form
// base-label.N or base-label.N+<anything> (semver build metadata).
func LatestPrereleaseNumber(root, base, label string) (int, error) {
	tags, err := run(root, "tag", "--sort=-v:refname")
	if err != nil {
		return 0, err
	}
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `-` + regexp.QuoteMeta(label) + `\.(\d+)(?:\+[0-9a-zA-Z.-]+)?$`)
	best := 0
	for _, tag := range strings.Split(tags, "\n") {
		if m := pattern.FindStringSubmatch(strings.TrimSpace(tag)); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > best {
				best = n
			}
		}
	}
	return best, nil
}

func ShortSHA(root string) (string, error) {
	return run(root, "rev-parse", "--short", "HEAD")
}

func CurrentBranch(root string) (string, error) {
	return run(root, "rev-parse", "--abbrev-ref", "HEAD")
}

func CurrentCommit(root string) (string, error) {
	return run(root, "rev-parse", "HEAD")
}

var safeTagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+-]*$`)

func CreateTag(root, tag string) error {
	if !safeTagRe.MatchString(tag) {
		return fmt.Errorf("refusing to create tag with unsafe name %q", tag)
	}
	_, err := run(root, "tag", "-a", "-m", tag, tag)
	return err
}

// TagExists reports whether tag already exists in the repo.
func TagExists(root, tag string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+tag)
	cmd.Dir = root
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("checking tag %q: %w", tag, err)
}

// IdentityConfigured reports whether a committer identity is resolvable from git
// config or the standard environment overrides. The CLI never sets one itself
// (only the workflow does), so a commit on an unconfigured machine would fail
// after the working tree was already mutated.
func IdentityConfigured(root string) bool {
	name, _ := run(root, "config", "user.name")
	email, _ := run(root, "config", "user.email")
	hasName := name != "" || os.Getenv("GIT_AUTHOR_NAME") != "" || os.Getenv("GIT_COMMITTER_NAME") != ""
	hasEmail := email != "" || os.Getenv("GIT_AUTHOR_EMAIL") != "" || os.Getenv("GIT_COMMITTER_EMAIL") != ""
	return hasName && hasEmail
}

// ResetHard moves the current branch to ref and resets the index and working
// tree to match, used to roll a release back to its starting commit after a
// partial failure.
func ResetHard(root, ref string) error {
	_, err := run(root, "reset", "--hard", ref)
	return err
}

func Add(root string, paths ...string) error {
	_, err := run(root, append([]string{"add"}, paths...)...)
	return err
}

func Commit(root, message string) error {
	_, err := run(root, "commit", "-m", message)
	return err
}

// MergeRefBaseParent returns HEAD's first parent and true when HEAD is a merge
// commit (two or more parents). A GitHub pull_request checkout uses the synthetic
// refs/pull/N/merge ref, whose first parent is the base branch tip the PR is
// being merged into — a more reliable base than the event's recorded base.sha,
// which can lag on long-lived or reopened PRs. Returns ("", false) otherwise.
func MergeRefBaseParent(root string) (string, bool) {
	out, err := run(root, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return "", false
	}
	// Output is "<commit> <parent1> <parent2> ..."; a merge has >= 2 parents.
	fields := strings.Fields(out)
	if len(fields) >= 3 {
		return fields[1], true
	}
	return "", false
}

// IsAncestor reports whether a is an ancestor of b (true also when a == b).
func IsAncestor(root, a, b string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir = root
	return cmd.Run() == nil
}

// ChangedChangeFilesSinceRef returns basenames of .changes/*.md files added
// since ref (exclusive) up to HEAD. When ref is empty, the full history is
// searched. Files matching readme.md or _headline.md are excluded.
func ChangedChangeFilesSinceRef(root, ref string) ([]string, error) {
	var args []string
	if ref != "" {
		args = []string{"log", ref + "..HEAD", "--diff-filter=A", "--name-only", "--format=", "--", ".changes/"}
	} else {
		args = []string{"log", "--diff-filter=A", "--name-only", "--format=", "--", ".changes/"}
	}
	out, err := run(root, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		base := filepath.Base(line)
		lower := strings.ToLower(base)
		if !strings.HasSuffix(lower, ".md") || lower == "readme.md" || lower == "_headline.md" {
			continue
		}
		if seen[base] {
			continue
		}
		seen[base] = true
		files = append(files, base)
	}
	return files, nil
}

// LatestTagMatching returns the name of the most recently created tag that
// matches any of the provided patterns. Returns "" when no tag matches.
func LatestTagMatching(root string, patterns []*regexp.Regexp) (string, error) {
	out, err := run(root, "tag", "--sort=-creatordate")
	if err != nil {
		return "", err
	}
	for _, tag := range strings.Split(out, "\n") {
		tag = strings.TrimSpace(tag)
		for _, re := range patterns {
			if re.MatchString(tag) {
				return tag, nil
			}
		}
	}
	return "", nil
}

// ContributorsSince returns the deduplicated commit-author logins (preserving
// first-seen order, case-insensitive dedup) between previousTag and HEAD via
// gh api. No filtering: bots and maintainers are included. previousTag must be
// a real ref (the literal tag, including prefix), not a bare version. Returns
// nil if gh is unavailable or previousTag is empty.
func ContributorsSince(root, previousTag string) []string {
	if previousTag == "" {
		return nil
	}
	out, err := ghAPI(root,
		fmt.Sprintf("repos/{owner}/{repo}/compare/%s...HEAD", previousTag),
		"--jq", "[.commits[].author.login | select(. != null)]")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch contributors (%v)\n", err)
		return nil
	}
	var logins []string
	if json.Unmarshal(out, &logins) != nil {
		return nil
	}
	seen := map[string]bool{}
	var res []string
	for _, l := range logins {
		key := strings.ToLower(l)
		if l == "" || seen[key] {
			continue
		}
		seen[key] = true
		res = append(res, l)
	}
	return res
}

// Maintainers returns the set of repo collaborator logins (lowercased) that
// have admin or maintain permission, used to exclude them from the
// contributors footer. Returns nil if gh is unavailable (e.g. the token lacks
// push access), in which case no maintainer filtering is applied.
func Maintainers(root string) map[string]bool {
	out, err := ghAPI(root, "repos/{owner}/{repo}/collaborators", "--paginate",
		"--jq", ".[] | select(.permissions.admin or .permissions.maintain) | .login")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch maintainers; contributors footer will not exclude them (%v)\n", err)
		return nil
	}
	set := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			set[strings.ToLower(l)] = true
		}
	}
	return set
}

// DerivePRNumber finds the PR that introduced .changes/<filename> via gh api.
// Returns 0 if gh is unavailable or the commit has no associated PR.
func DerivePRNumber(root, filename string) int {
	sha, err := run(root, "log", "--diff-filter=A", "--format=%H", "-n", "1", "--", ".changes/"+filename)
	if err != nil || sha == "" {
		return 0
	}
	out, err := ghAPI(root, fmt.Sprintf("repos/{owner}/{repo}/commits/%s/pulls", sha))
	if err != nil {
		return 0
	}
	var pulls []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(out, &pulls); err != nil || len(pulls) == 0 {
		return 0
	}
	return pulls[0].Number
}

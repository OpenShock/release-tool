package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

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

func CreateTag(root, tag string) error {
	_, err := run(root, "tag", "-a", "-m", tag, tag)
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

// DerivePRNumber finds the PR that introduced .changes/<filename> via gh api.
// Returns 0 if gh is unavailable or the commit has no associated PR.
func DerivePRNumber(root, filename string) int {
	sha, err := run(root, "log", "--diff-filter=A", "--format=%H", "-n", "1", "--", ".changes/"+filename)
	if err != nil || sha == "" {
		return 0
	}
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/commits/%s/pulls", sha))
	cmd.Dir = root
	out, err := cmd.Output()
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

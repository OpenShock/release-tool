package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
)

var bumpOrder = map[string]int{"patch": 0, "minor": 1, "major": 2}

func HighestBump(ch []*changes.Change) string {
	best := "patch"
	for _, c := range ch {
		if bumpOrder[c.Bump] > bumpOrder[best] {
			best = c.Bump
		}
	}
	return best
}

// versionRe matches a leading MAJOR.MINOR.PATCH. It deliberately prefix-matches:
// any trailing suffix (a semver pre-release/build tag like "-rc.1", or stray
// text) is ignored and only the numeric core is captured. ParseVersion is only
// called on the stable-version capture from LatestStableTag, which is already
// clean, so this leniency is never exercised on real input. The contract is
// pinned by TestParseVersion.
var versionRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

func ParseVersion(s string) (maj, min, pat int, err error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("invalid version: %q", s)
	}
	maj, _ = strconv.Atoi(m[1])
	min, _ = strconv.Atoi(m[2])
	pat, _ = strconv.Atoi(m[3])
	return
}

func BumpVersion(maj, min, pat int, bump string) (int, int, int) {
	switch bump {
	case "major":
		return maj + 1, 0, 0
	case "minor":
		return maj, min + 1, 0
	default:
		return maj, min, pat + 1
	}
}

func ComputeNext(ch []*changes.Change, latest string) (string, error) {
	maj, min, pat := 0, 0, 0
	if latest != "" {
		var err error
		maj, min, pat, err = ParseVersion(latest)
		if err != nil {
			return "", err
		}
	}
	maj, min, pat = BumpVersion(maj, min, pat, HighestBump(ch))
	return fmt.Sprintf("%d.%d.%d", maj, min, pat), nil
}

var exactVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// OverrideVersion validates a manually chosen stable version, given with or
// without the tag prefix, against next, the version the pending changes call
// for. It may skip ahead of next (e.g. past a tag name GitHub will no longer
// accept) but never below it, so the bump the changes require is kept.
func OverrideVersion(v, tagPrefix, next string) (string, error) {
	if tagPrefix != "" {
		v = strings.TrimPrefix(v, tagPrefix)
	}
	if !exactVersionRe.MatchString(v) {
		return "", fmt.Errorf("version override %q must be MAJOR.MINOR.PATCH", v)
	}
	if versionLess(v, next) {
		return "", fmt.Errorf("version override %s is below %s, which the pending changes require", v, next)
	}
	return v, nil
}

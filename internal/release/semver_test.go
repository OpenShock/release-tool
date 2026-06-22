package release

import (
	"testing"

	"github.com/OpenShock/release-tool/internal/changes"
)

func TestHighestBump(t *testing.T) {
	cases := []struct {
		name  string
		bumps []string
		want  string
	}{
		{"single patch", []string{"patch"}, "patch"},
		{"single minor", []string{"minor"}, "minor"},
		{"single major", []string{"major"}, "major"},
		{"patch+minor", []string{"patch", "minor"}, "minor"},
		{"all three", []string{"patch", "major", "minor"}, "major"},
		{"duplicates", []string{"minor", "minor"}, "minor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ch []*changes.Change
			for _, b := range tc.bumps {
				ch = append(ch, &changes.Change{Bump: b})
			}
			if got := HighestBump(ch); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		input   string
		maj     int
		min     int
		pat     int
		wantErr bool
	}{
		{"1.2.3", 1, 2, 3, false},
		{"0.0.0", 0, 0, 0, false},
		{"10.20.30", 10, 20, 30, false},
		{"1.2.3-rc.1", 1, 2, 3, false}, // prefix match - semver pre-release suffix is ignored
		{"1.2.3foo", 1, 2, 3, false},   // prefix match - trailing text is ignored, not an error
		{"1.2.3.4", 1, 2, 3, false},    // prefix match - only the first three components are captured
		{"abc", 0, 0, 0, true},
		{"1.2", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			maj, min, pat, err := ParseVersion(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if maj != tc.maj || min != tc.min || pat != tc.pat {
				t.Errorf("got %d.%d.%d, want %d.%d.%d", maj, min, pat, tc.maj, tc.min, tc.pat)
			}
		})
	}
}

func TestBumpVersion(t *testing.T) {
	cases := []struct {
		maj, min, pat int
		bump          string
		wantMaj       int
		wantMin       int
		wantPat       int
	}{
		{1, 2, 3, "patch", 1, 2, 4},
		{1, 2, 3, "minor", 1, 3, 0},
		{1, 2, 3, "major", 2, 0, 0},
		{0, 0, 0, "patch", 0, 0, 1},
		{0, 0, 0, "minor", 0, 1, 0},
		{0, 0, 0, "major", 1, 0, 0},
		{1, 2, 3, "unknown", 1, 2, 4}, // unknown defaults to patch
	}
	for _, tc := range cases {
		t.Run(tc.bump, func(t *testing.T) {
			gotMaj, gotMin, gotPat := BumpVersion(tc.maj, tc.min, tc.pat, tc.bump)
			if gotMaj != tc.wantMaj || gotMin != tc.wantMin || gotPat != tc.wantPat {
				t.Errorf("got %d.%d.%d, want %d.%d.%d", gotMaj, gotMin, gotPat, tc.wantMaj, tc.wantMin, tc.wantPat)
			}
		})
	}
}

func TestComputeNext(t *testing.T) {
	patch := []*changes.Change{{Bump: "patch"}}
	minor := []*changes.Change{{Bump: "minor"}}
	major := []*changes.Change{{Bump: "major"}}

	cases := []struct {
		name    string
		changes []*changes.Change
		latest  string
		want    string
		wantErr bool
	}{
		{"patch bump", patch, "1.2.3", "1.2.4", false},
		{"minor bump", minor, "1.2.3", "1.3.0", false},
		{"major bump", major, "1.2.3", "2.0.0", false},
		// bootstrap: no existing stable tag
		{"bootstrap patch", patch, "", "0.0.1", false},
		{"bootstrap minor", minor, "", "0.1.0", false},
		{"bootstrap major", major, "", "1.0.0", false},
		// bad latest tag
		{"bad tag", patch, "notaversion", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeNext(tc.changes, tc.latest)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

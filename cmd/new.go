package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/spf13/cobra"
)

var (
	newType       string
	newBreaking   bool
	newCategories []string
)

var newCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new change file in .changes/",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		breakingSet := cmd.Flags().Changed("breaking")
		return runNew(args, breakingSet)
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newType, "type", "t", "", "Bump type: major, minor, or patch (required)")
	newCmd.Flags().BoolVarP(&newBreaking, "breaking", "b", false, "Mark as breaking (implied for major)")
	newCmd.Flags().StringSliceVarP(&newCategories, "categories", "c", nil, "Categories (comma-separated)")
	_ = newCmd.MarkFlagRequired("type")
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(title)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = strings.TrimRight(s[:50], "-")
	}
	return s
}

func runNew(args []string, breakingSet bool) error {
	title := strings.TrimSpace(args[0])
	if title == "" {
		return fmt.Errorf("title must not be empty")
	}
	switch newType {
	case "major", "minor", "patch":
	default:
		return fmt.Errorf("--type must be major, minor, or patch")
	}

	root := projectRoot()
	slug := slugify(title)
	if slug == "" {
		return fmt.Errorf("could not derive a valid filename from title %q", title)
	}

	dir := filepath.Join(root, changes.Dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", changes.Dir, err)
	}

	path := filepath.Join(dir, slug+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s", filepath.Join(changes.Dir, slug+".md"))
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: %s\n", newType)
	// Only write breaking when it differs from the type's default.
	// major defaults to true, minor/patch default to false.
	// Only emit the field when the user explicitly set --breaking.
	if breakingSet {
		if newType == "major" && !newBreaking {
			b.WriteString("breaking: false\n")
		} else if newType != "major" && newBreaking {
			b.WriteString("breaking: true\n")
		}
	}
	if len(newCategories) > 0 {
		fmt.Fprintf(&b, "categories: [%s]\n", strings.Join(newCategories, ", "))
	}
	b.WriteString("---\n")
	fmt.Fprintf(&b, "%s\n", title)

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing change file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created %s\n", filepath.Join(changes.Dir, slug+".md"))
	return nil
}

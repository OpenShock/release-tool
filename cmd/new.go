package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/OpenShock/release-tool/internal/changes"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	newType       string
	newBreaking   bool
	newCategories []string
)

var newCmd = &cobra.Command{
	Use:   "new [title]",
	Short: "Create a new change file in .changes/",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isCI() {
			return runNewCI(cmd, args)
		}
		return runNewInteractive(args)
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newType, "type", "t", "", "Bump type: major, minor, or patch (required in CI)")
	newCmd.Flags().BoolVarP(&newBreaking, "breaking", "b", false, "Mark as breaking (implied for major)")
	newCmd.Flags().StringSliceVarP(&newCategories, "categories", "c", nil, "Categories (comma-separated)")
}

func isCI() bool {
	return os.Getenv("CI") != ""
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

func runNewCI(cmd *cobra.Command, args []string) error {
	if newType == "" {
		return fmt.Errorf("--type is required in CI (major, minor, or patch)")
	}
	title := ""
	if len(args) > 0 {
		title = strings.TrimSpace(args[0])
	}
	if title == "" {
		return fmt.Errorf("title argument is required in CI")
	}
	breakingSet := cmd.Flags().Changed("breaking")
	return writeChangeFile(title, newType, newBreaking, breakingSet, newCategories)
}

func runNewInteractive(args []string) error {
	title := ""
	if len(args) > 0 {
		title = strings.TrimSpace(args[0])
	}

	bumpType := newType
	breaking := false
	categoriesInput := ""

	fields := []huh.Field{}

	if title == "" {
		fields = append(fields, huh.NewInput().
			Title("Change title").
			Placeholder("e.g. Add support for X").
			Value(&title))
	}

	if bumpType == "" {
		fields = append(fields, huh.NewSelect[string]().
			Title("Bump type").
			Options(
				huh.NewOption("patch — backwards-compatible bug fix", "patch"),
				huh.NewOption("minor — new backwards-compatible feature", "minor"),
				huh.NewOption("major — breaking change", "major"),
			).
			Value(&bumpType))
	}

	fields = append(fields,
		huh.NewInput().
			Title("Categories (optional, comma-separated)").
			Placeholder("e.g. api, cli").
			Value(&categoriesInput),
	)

	form := huh.NewForm(huh.NewGroup(fields...))
	if err := form.Run(); err != nil {
		return err
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title must not be empty")
	}

	// For major, ask about breaking only if user might want to opt out.
	// For minor/patch, ask only if they want to mark as breaking.
	breakingSet := false
	if bumpType == "major" {
		breaking = true
		var optOut bool
		confirm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Mark as breaking?").
				Value(&optOut).
				Affirmative("Yes").
				Negative("No"),
		))
		_ = confirm.Run()
		breaking = optOut
		breakingSet = true
	} else {
		var wantBreaking bool
		confirm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Mark as breaking?").
				Value(&wantBreaking).
				Affirmative("Yes").
				Negative("No"),
		))
		_ = confirm.Run()
		if wantBreaking {
			breaking = true
			breakingSet = true
		}
	}

	var cats []string
	if categoriesInput != "" {
		for _, c := range strings.Split(categoriesInput, ",") {
			if trimmed := strings.TrimSpace(c); trimmed != "" {
				cats = append(cats, trimmed)
			}
		}
	}

	return writeChangeFile(title, bumpType, breaking, breakingSet, cats)
}

func writeChangeFile(title, bumpType string, breaking, breakingSet bool, categories []string) error {
	switch bumpType {
	case "major", "minor", "patch":
	default:
		return fmt.Errorf("type must be major, minor, or patch")
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
	fmt.Fprintf(&b, "type: %s\n", bumpType)
	if breakingSet {
		if bumpType == "major" && !breaking {
			b.WriteString("breaking: false\n")
		} else if bumpType != "major" && breaking {
			b.WriteString("breaking: true\n")
		}
	}
	if len(categories) > 0 {
		fmt.Fprintf(&b, "categories: [%s]\n", strings.Join(categories, ", "))
	}
	b.WriteString("---\n")
	fmt.Fprintf(&b, "%s\n", title)

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing change file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created %s\n", filepath.Join(changes.Dir, slug+".md"))
	return nil
}

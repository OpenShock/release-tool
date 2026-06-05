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
	newKind     string
	newBreaking bool
	newMandatory bool
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
	newCmd.Flags().StringVarP(&newKind, "kind", "k", "", "Change kind: added, changed, deprecated, removed, fixed, security (required in CI)")
	newCmd.Flags().BoolVarP(&newBreaking, "breaking", "b", false, "Mark as breaking change (major semver bump)")
	newCmd.Flags().BoolVar(&newMandatory, "mandatory", false, "Mark as a mandatory version (must be installed before newer versions)")
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

var validKinds = []string{"added", "changed", "deprecated", "removed", "fixed", "security", "safety", "chore"}

func isValidKind(k string) bool {
	for _, v := range validKinds {
		if k == v {
			return true
		}
	}
	return false
}

func runNewCI(cmd *cobra.Command, args []string) error {
	if newKind == "" {
		return fmt.Errorf("--kind is required in CI (added, changed, deprecated, removed, fixed, security)")
	}
	if !isValidKind(newKind) {
		return fmt.Errorf("invalid kind %q (must be added, changed, deprecated, removed, fixed, or security)", newKind)
	}
	title := ""
	if len(args) > 0 {
		title = strings.TrimSpace(args[0])
	}
	if title == "" {
		return fmt.Errorf("title argument is required in CI")
	}
	return writeChangeFile(title, newKind, newBreaking, newMandatory)
}

func runNewInteractive(args []string) error {
	title := ""
	if len(args) > 0 {
		title = strings.TrimSpace(args[0])
	}

	kind := newKind
	breaking := newBreaking
	mandatory := newMandatory

	fields := []huh.Field{}

	if title == "" {
		fields = append(fields, huh.NewInput().
			Title("Change title").
			Placeholder("e.g. Add support for X").
			Value(&title))
	}

	if kind == "" {
		fields = append(fields, huh.NewSelect[string]().
			Title("Change kind").
			Options(
				huh.NewOption("added - new feature or capability", "added"),
				huh.NewOption("changed - change to existing functionality", "changed"),
				huh.NewOption("deprecated - feature marked for removal", "deprecated"),
				huh.NewOption("removed - feature removed", "removed"),
				huh.NewOption("fixed - bug fix", "fixed"),
				huh.NewOption("security - security vulnerability fix", "security"),
				huh.NewOption("safety - physical safety improvement (e.g. e-stop)", "safety"),
				huh.NewOption("chore - dependency bump, CI update, refactor", "chore"),
			).
			Value(&kind))
	}

	form := huh.NewForm(huh.NewGroup(fields...))
	if err := form.Run(); err != nil {
		return err
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title must not be empty")
	}

	if !breaking {
		var wantBreaking bool
		confirm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Mark as breaking?").
				Value(&wantBreaking).
				Affirmative("Yes").
				Negative("No"),
		))
		_ = confirm.Run()
		breaking = wantBreaking
	}

	return writeChangeFile(title, kind, breaking, mandatory)
}

func writeChangeFile(title, kind string, breaking, mandatory bool) error {
	if !isValidKind(kind) {
		return fmt.Errorf("kind must be added, changed, deprecated, removed, fixed, or security")
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
	fmt.Fprintf(&b, "kind: %s\n", kind)
	if breaking {
		b.WriteString("breaking: true\n")
	}
	if mandatory {
		b.WriteString("mandatory: true\n")
	}
	b.WriteString("---\n")
	fmt.Fprintf(&b, "%s\n", title)

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing change file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created %s\n", filepath.Join(changes.Dir, slug+".md"))
	return nil
}

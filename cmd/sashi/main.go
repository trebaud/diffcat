package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/trebaud/sashi/internal/git"
	"github.com/trebaud/sashi/internal/tui"
)

// version is set at build time via -ldflags "-X main.ldflagsVersion=$(git describe --tags)",
// falling back to the module version embedded by `go install ...@tag`.
var version = resolveVersion()

var ldflagsVersion = "dev"

func resolveVersion() string {
	if v := strings.TrimPrefix(ldflagsVersion, "v"); v != "" && v != "dev" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimPrefix(bi.Main.Version, "v"); v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var base string

	root := &cobra.Command{
		Use:           "sashi [path]",
		Short:         "Visualize the git diff of the current branch against master",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return tui.Run(dir, base)
		},
	}
	root.PersistentFlags().StringVarP(&base, "base", "b", "", "Base ref to diff against (default: auto-detect master/main)")
	root.SetVersionTemplate("sashi v{{.Version}}\n")
	root.Flags().BoolP("version", "v", false, "Show version")

	root.AddCommand(filesCmd(&base))
	return root
}

// filesCmd is a non-interactive listing useful for scripting and quick checks.
func filesCmd(base *string) *cobra.Command {
	return &cobra.Command{
		Use:   "files [path]",
		Short: "List changed files against the base branch (non-interactive)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			repo, err := git.RepoRoot(dir)
			if err != nil {
				return err
			}
			b := *base
			if b == "" {
				b = git.DefaultBranch(repo)
			}
			ref := git.BaseRef(repo, b)
			files, err := git.ChangedFiles(repo, ref)
			if err != nil {
				return err
			}
			for _, f := range files {
				if f.Binary() {
					fmt.Printf("%s\t%s\t(binary)\n", f.Status, f.Path)
				} else {
					fmt.Printf("%s\t%s\t+%d/-%d\n", f.Status, f.Path, f.Added, f.Deleted)
				}
			}
			return nil
		},
	}
}

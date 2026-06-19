// Package tui renders diffcat's interactive terminal UI following the Elm
// architecture. Each concern lives in its own file:
//
//   - model.go   state, Init, helpers
//   - update.go  Update + key handling
//   - view.go    View + diff rendering
//   - theme.go   colors and styles
package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// Run computes the diff of the repo at dir against baseName and launches the
// interactive viewer. baseName may be empty to auto-detect (master → main). opts
// carries command-line overrides for theme/icons/motion; they're layered over
// the env, the saved config, and terminal auto-detection by resolveOptions.
func Run(dir, baseName string, opts Options) error {
	repo, err := git.RepoRoot(dir)
	if err != nil {
		return err
	}

	defaultBranch := git.DefaultBranch(repo)
	if baseName == "" {
		baseName = defaultBranch
	}
	baseIsDefault := baseName == defaultBranch
	base := git.BaseRef(repo, baseName)

	files, err := git.ChangedFiles(repo, base)
	if err != nil {
		return err
	}
	branch := git.CurrentBranch(repo)
	shortstat := git.Shortstat(repo, base)

	r := resolveOptions(opts, loadConfig())
	ApplyTheme(themes[r.themeIdx], r.dark)

	p := tea.NewProgram(newModel(repo, base, baseName, baseIsDefault, branch, files, shortstat, r))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
	return nil
}

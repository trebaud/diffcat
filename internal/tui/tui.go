// Package tui renders sashi's interactive terminal UI following the Elm
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

	"github.com/trebaud/sashi/internal/git"
)

// Run computes the diff of the repo at dir against baseName and launches the
// interactive viewer. baseName may be empty to auto-detect (master → main).
func Run(dir, baseName string) error {
	repo, err := git.RepoRoot(dir)
	if err != nil {
		return err
	}

	if baseName == "" {
		baseName = git.DefaultBranch(repo)
	}
	base := git.BaseRef(repo, baseName)

	files, err := git.ChangedFiles(repo, base)
	if err != nil {
		return err
	}
	branch := git.CurrentBranch(repo)
	shortstat := git.Shortstat(repo, base)

	dark := DetectAndApplyTheme()

	p := tea.NewProgram(newModel(repo, base, baseName, branch, files, shortstat, dark))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
	return nil
}

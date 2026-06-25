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
	"sync"

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

	// Only the cheap ref resolution above runs before launch (each ~ms even on a
	// huge repo). The expensive change-list / history / status work is deferred to
	// a background command (see Init → startupCmd) so the first frame — a loading
	// screen — paints immediately instead of after a multi-second blank stall.
	r := resolveOptions(opts, loadConfig())
	ApplyTheme(themes[r.themeIdx], r.dark)

	p := tea.NewProgram(newModel(repo, base, baseName, baseIsDefault, r))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
	return nil
}

// startup is the git state gathered at launch, before the first frame paints.
// All of it is computed concurrently by gatherStartup so cold-start wall time is
// the slowest single call rather than their sum.
type startup struct {
	files        []git.FileChange // branch-vs-base change list (the `D` tree + header shortstat)
	branch       string           // current branch name
	fingerprint  string           // git.Fingerprint seed for the background sync poll
	commits      []git.Commit     // history list (base..HEAD + base context)
	baseStart    int              // index where base history begins, or -1
	workingCount int              // uncommitted-change count (the working-tree row)
}

// gatherStartup runs the independent launch git calls concurrently. Each shells a
// separate git process and reads its own refs/objects, so they don't contend; the
// only ordering constraint (resolving base) is already done by the caller. Errors
// degrade to zero values — the same fallback each call made when run serially.
func gatherStartup(repo, base, baseName string) startup {
	var su startup
	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); su.files, _ = git.ChangedFiles(repo, base) }()
	go func() { defer wg.Done(); su.branch = git.CurrentBranch(repo) }()
	go func() { defer wg.Done(); su.fingerprint = git.Fingerprint(repo, baseName) }()
	go func() { defer wg.Done(); su.commits, su.baseStart, _ = git.BranchHistory(repo, base, baseHistoryLimit) }()
	go func() { defer wg.Done(); su.workingCount = git.WorkingCount(repo) }()
	wg.Wait()
	return su
}

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
// interactive viewer. baseName may be empty to auto-detect (master → main); it is
// a label, resolved to the ref actually diffed against by git.BaseBranchRev. opts
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
	// The label the reader typed (or the repo default) is not necessarily the ref to
	// measure against: a local master that hasn't been fetched lately would report
	// everything that landed on master since as part of this branch. BaseBranchRev
	// picks the tighter of the local branch and its remote-tracking counterpart.
	baseRev := git.BaseBranchRev(repo, baseName)
	base := git.BaseRef(repo, baseRev)

	// Only the cheap ref resolution above runs before launch (each ~ms even on a
	// huge repo). The expensive change-list / history / status work is deferred to
	// a background command (see Init → startupCmd) so the first frame — a loading
	// screen — paints immediately instead of after a multi-second blank stall.
	r := resolveOptions(opts, loadConfig())
	ApplyTheme(themes[r.themeIdx], r.dark)

	p := tea.NewProgram(newModel(repo, base, baseName, baseRev, baseIsDefault, r))
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
	commits      []git.Commit     // history list (base..HEAD + base context), HEAD side capped
	baseStart    int              // index where base history begins, or -1
	historyTrunc bool             // the capped HEAD walk hit its limit — more commits exist to backfill
	workingCount int              // uncommitted-change count (the working-tree row)
}

// gatherStartup runs the independent launch git calls concurrently. Each shells a
// separate git process and reads its own refs/objects, so they don't contend; the
// only ordering constraint (resolving base) is already done by the caller. Errors
// degrade to zero values — the same fallback each call made when run serially.
func gatherStartup(repo, base, baseRev string) startup {
	var su startup
	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); su.files, _ = git.ChangedFiles(repo, base) }()
	go func() { defer wg.Done(); su.branch = git.CurrentBranch(repo) }()
	go func() { defer wg.Done(); su.fingerprint = git.Fingerprint(repo, baseRev) }()
	go func() {
		defer wg.Done()
		// Cap the HEAD-side walk so the first list paints fast; the full list is
		// backfilled in the background (see applyStartup → fullHistoryCmd).
		su.commits, su.baseStart, _ = git.BranchHistory(repo, base, baseHistoryLimit, initialHistoryLimit)
		su.historyTrunc = headCount(su.commits, su.baseStart) >= initialHistoryLimit
	}()
	go func() { defer wg.Done(); su.workingCount = git.WorkingCount(repo) }()
	wg.Wait()
	return su
}

// headCount returns how many of the loaded commits are HEAD-side (the branch's own
// commits, or all of them in the base-branch case) — i.e. the part the headLimit
// cap applies to. baseStart marks where the base-context tail begins, or -1 when
// there is none. Used to tell whether the capped launch walk was truncated.
func headCount(commits []git.Commit, baseStart int) int {
	if baseStart >= 0 {
		return baseStart
	}
	return len(commits)
}

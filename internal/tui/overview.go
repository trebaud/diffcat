package tui

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// overview.go holds the overview dashboard (viewOverview, toggled with `S`): a
// full-screen, GitHub-style summary — totals, a per-file churn list with
// proportional add/del bars, and a language breakdown. It has two scopes: the
// whole branch-vs-base diff (the default), or a single commit picked from the
// history. Entering/leaving the mode and the cursor live here; rendering is in
// overview_view.go.

// enterOverview switches into the branch-vs-base dashboard. Commits are loaded
// (if not already) so the summary can show the branch's commit count. The
// AI/human split can scan the whole history, so it's computed in the background
// (see ensureAuthorship) and the returned command fills in the bars when ready —
// the dashboard opens instantly meanwhile.
func (m *model) enterOverview() tea.Cmd {
	if len(m.commits) == 0 {
		m.loadCommits()
	}
	m.overviewCommit = nil
	m.overviewCommitFiles = nil
	m.overviewReturn = viewBranch
	m.mode = viewOverview
	m.focus = focusFiles
	m.overviewCursor = 0
	return m.ensureAuthorship()
}

// enterCommitOverview opens the dashboard scoped to a single commit: it
// summarizes that commit's changed files instead of the branch diff. The
// commit-count and AI/human-split rows (meaningful only across a range) give way
// to the commit's SHA/subject and a single AI-or-human author label. Returns to
// wherever it was opened from (the history list or the per-commit tree).
//
// When there's no single commit in scope (the history's working-tree row, or a
// working-tree drill-in), there's nothing to scope to, so it falls back to the
// branch-wide summary — including the AI/human split chart — still returning to
// the view it was opened from.
func (m *model) enterCommitOverview() tea.Cmd {
	c := m.detailsCommit()
	if c == nil {
		origin := m.mode
		cmd := m.enterOverview()
		m.overviewReturn = origin
		return cmd
	}
	files, err := git.CommitFiles(m.repo, c.SHA)
	if err != nil {
		files = nil
	}
	m.overviewCommit = c
	m.overviewCommitFiles = files
	m.overviewReturn = m.mode
	m.mode = viewOverview
	m.focus = focusFiles
	m.overviewCursor = 0
	return nil
}

// exitOverview leaves the dashboard for the view it was opened from: the
// branch-vs-base diff, the history list, or the per-commit tree, restoring that
// view's diff.
func (m *model) exitOverview() {
	ret := m.overviewReturn
	m.overviewCommit = nil
	m.overviewCommitFiles = nil
	m.focus = focusFiles
	switch ret {
	case viewLog:
		m.mode = viewLog
		m.loadCommitDiff()
	case viewCommit:
		m.mode = viewCommit
		m.loadDiff()
	default:
		m.mode = viewBranch
		m.loadDiff()
	}
}

// moveOverviewCursor moves the dashboard's file selection, clamped to the list.
func (m *model) moveOverviewCursor(delta int) {
	n := len(m.overviewFileSet())
	if n == 0 {
		return
	}
	m.overviewCursor += delta
	if m.overviewCursor < 0 {
		m.overviewCursor = 0
	}
	if m.overviewCursor >= n {
		m.overviewCursor = n - 1
	}
}

// enterOverviewFile drills the highlighted dashboard file into a diff view: the
// branch overview opens it in the branch-vs-base tree; a commit overview opens
// it in that commit's per-file tree. Either way it leaves the overview and jumps
// the tree cursor to that file (expanding any collapsed ancestors).
func (m *model) enterOverviewFile() {
	files := m.overviewFiles()
	if m.overviewCursor < 0 || m.overviewCursor >= len(files) {
		m.exitOverview()
		return
	}
	path := files[m.overviewCursor].Path
	if c := m.overviewCommit; c != nil {
		// Commit scope: land in that commit's per-file tree (viewCommit). If we
		// came from the history list we still need to scope into it; if we came
		// from the per-commit tree, m.files already holds the commit's files.
		origin := m.overviewReturn
		m.overviewCommit = nil
		m.overviewCommitFiles = nil
		if origin == viewCommit {
			m.mode = viewCommit
		} else {
			m.scopeToCommit(c)
		}
		m.jumpToFile(path)
		return
	}
	m.mode = viewBranch
	m.jumpToFile(path)
}

// overviewFileSet is the file list the dashboard summarizes: the in-scope
// commit's files for a commit overview, else the branch-vs-base list (m.files).
func (m model) overviewFileSet() []git.FileChange {
	if m.overviewCommit != nil {
		return m.overviewCommitFiles
	}
	return m.files
}

// overviewFiles returns the in-scope changed files sorted by total churn
// (added+deleted) descending, then path — the order the dashboard lists and the
// cursor indexes.
func (m model) overviewFiles() []git.FileChange {
	out := append([]git.FileChange(nil), m.overviewFileSet()...)
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := churnOf(out[i]), churnOf(out[j])
		if ci != cj {
			return ci > cj
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// churnOf is a file's total changed-line count (added + deleted), treating the
// -1 binary sentinels as 0 so binaries sort to the bottom with no bar.
func churnOf(f git.FileChange) int {
	c := 0
	if f.Added > 0 {
		c += f.Added
	}
	if f.Deleted > 0 {
		c += f.Deleted
	}
	return c
}

// langStat is one language's aggregate churn for the breakdown section.
type langStat struct {
	name  string
	churn int
}

// languageStats aggregates changed-line counts per language across the files,
// keyed by the Chroma lexer name for each path (so coverage matches the syntax
// highlighter), sorted by churn descending. Binary and zero-churn files are
// skipped; files with no matching lexer fall under "other".
func languageStats(files []git.FileChange) []langStat {
	byLang := map[string]int{}
	for _, f := range files {
		c := churnOf(f)
		if c == 0 {
			continue
		}
		name := "other"
		if lx := lexerFor(f.Path); lx != nil {
			name = lx.Config().Name
		}
		byLang[name] += c
	}
	out := make([]langStat, 0, len(byLang))
	for n, c := range byLang {
		out = append(out, langStat{name: n, churn: c})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].churn != out[j].churn {
			return out[i].churn > out[j].churn
		}
		return out[i].name < out[j].name
	})
	return out
}

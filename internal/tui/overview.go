package tui

import (
	"sort"

	"github.com/trebaud/diffcat/internal/git"
)

// overview.go holds the branch overview dashboard (viewOverview, toggled with
// `S`): a full-screen, GitHub-style summary of the branch-vs-base diff — totals,
// a per-file churn list with proportional add/del bars, and a language
// breakdown. Entering/leaving the mode and the cursor live here; rendering is in
// overview_view.go.

// enterOverview switches into the dashboard. Commits are loaded (if not already)
// so the summary can show the branch's commit count.
func (m *model) enterOverview() {
	if len(m.commits) == 0 {
		m.loadCommits()
	}
	m.mode = viewOverview
	m.focus = focusFiles
	m.overviewCursor = 0
}

// exitOverview returns to the default branch-vs-base view, restoring the diff
// for the file under the tree cursor.
func (m *model) exitOverview() {
	m.mode = viewBranch
	m.focus = focusFiles
	m.loadDiff()
}

// moveOverviewCursor moves the dashboard's file selection, clamped to the list.
func (m *model) moveOverviewCursor(delta int) {
	n := len(m.files)
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

// enterOverviewFile drills the highlighted dashboard file into the normal branch
// diff view: it leaves the overview and jumps the tree cursor to that file
// (expanding any collapsed ancestors), showing its diff.
func (m *model) enterOverviewFile() {
	files := m.overviewFiles()
	if m.overviewCursor < 0 || m.overviewCursor >= len(files) {
		m.exitOverview()
		return
	}
	path := files[m.overviewCursor].Path
	m.mode = viewBranch
	m.jumpToFile(path)
}

// overviewFiles returns the changed files sorted by total churn (added+deleted)
// descending, then path — the order the dashboard lists and the cursor indexes.
func (m model) overviewFiles() []git.FileChange {
	out := append([]git.FileChange(nil), m.files...)
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

package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/alecthomas/chroma/v2"

	"github.com/trebaud/sashi/internal/diff"
	"github.com/trebaud/sashi/internal/git"
)

// focusPane identifies which of the two panes vim motions (j/k/gg/G) act on.
type focusPane int

const (
	focusFiles focusPane = iota // left: the changed-file list
	focusDiff                   // right: the unified diff
)

// viewMode selects the screen layout. viewBranch is the default file-tree/diff
// view; viewLog replaces the left pane with the branch's commit history and the
// right pane with the highlighted commit's full diff.
type viewMode int

const (
	viewBranch viewMode = iota // file tree (left) + selected file's diff (right)
	viewLog                    // commit list (left) + selected commit's diff (right)
	viewCommit                 // one commit's file tree (left) + its per-file diff (right)
)

// model is the Elm-architecture state for the diff viewer. The left pane lists
// changed files; the right pane shows the selected file's diff. Diff scrolling
// is tracked manually via diffOffset rather than a viewport component, matching
// the rest of the rendering which is hand-laid-out.
type model struct {
	mode     viewMode  // viewBranch (file/diff) or viewLog (history)
	focus    focusPane // pane that j/k/gg/G operate on
	pendingG bool      // first half of the `gg` chord was pressed

	repo      string
	base      string // ref the diff is computed against (merge base of master/HEAD)
	baseName  string // human label for the base branch
	branch    string // current branch
	shortstat string

	files []git.FileChange // raw change list, source of truth for the tree

	// The file list renders as a collapsible tree. rows is the flattened set of
	// visible lines (folders + files, collapsed branches omitted); cursor indexes
	// into it. collapsed remembers which folder paths are folded, surviving a
	// rebuild so a refresh doesn't re-expand everything.
	rows      []treeRow
	cursor    int
	collapsed map[string]bool

	// History (viewLog). commits is base..HEAD newest-first; commitCursor indexes
	// it (the list scrolls to keep it visible, like the file tree). The
	// highlighted commit's parsed diff is memoized by SHA so scrolling the list
	// doesn't re-shell `git show`.
	commits         []git.Commit
	commitCursor    int
	commitDiffCache map[string][]diff.Line

	// Per-commit drill-in (viewCommit). scopeCommit is the commit whose files the
	// tree currently shows (nil outside viewCommit); its SHA scopes loadDiff to
	// that commit's patch. Drilling in repurposes the shared tree fields, so the
	// branch tree is stashed in branchFiles/branchRows/branchCursor and restored
	// verbatim on the way back to the history list.
	scopeCommit  *git.Commit
	branchFiles  []git.FileChange
	branchRows   []treeRow
	branchCursor int

	// Pristine parsed diff for the selected file (never mutated), the new-side
	// file content, and the hidden-context gaps within it. revealed records how
	// far each gap has been expanded ([fromTop, fromBottom] per gap index). diff
	// stays pristine; viewLines is the derived display list — pristine lines
	// interleaved with revealed context and expand affordances — and splitRows is
	// its side-by-side projection. lineDigits sizes the line-number gutter.
	diff       []diff.Line
	fileLines  []string
	gaps       []diff.Gap
	revealed   map[int][2]int
	viewLines  []diff.Line
	splitRows  []diff.Row
	splitView  bool // false = unified (GitHub inline), true = side-by-side
	lineDigits int
	diffOffset int
	diffCursor int // selected row in the diff pane (index into viewLines/splitRows)

	// Syntax highlighting for the selected file: a lexer chosen from its path and
	// a per-line span cache (reset on every loadDiff so it tracks the lexer).
	// pathLexers memoizes per-path lexers for the combined commit-history preview,
	// whose multi-file patch can't ride a single m.lexer.
	lexer      chroma.Lexer
	hlCache    map[string][]span
	pathLexers map[string]chroma.Lexer

	width  int
	height int

	dark bool // current theme; toggled with `t`, seeds the style table on rebuild

	animFrame int // drives the nyan cat's leg/face wiggle

	showHelp bool
	err      error
}

func newModel(repo, base, baseName, branch string, files []git.FileChange, shortstat string, dark bool) model {
	m := model{
		repo:            repo,
		base:            base,
		baseName:        baseName,
		branch:          branch,
		shortstat:       shortstat,
		files:           files,
		collapsed:       map[string]bool{},
		commitDiffCache: map[string][]diff.Line{},
		dark:            dark,
	}
	m.rebuildTree()
	m.loadDiff()
	// With no working-tree changes the branch diff is empty (e.g. a clean
	// checkout of the default branch), so open straight into the commit history
	// — that's the only thing worth looking at.
	if len(files) == 0 {
		m.enterLog()
	}
	return m
}

// rebuildTree regenerates the flattened tree rows from the current file list,
// preserving folded folders. It clamps the cursor so it never dangles past the
// (possibly shorter) row set after files change.
func (m *model) rebuildTree() {
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.rows = nil
	flattenTree(buildTree(m.files), m.collapsed, 0, nil, &m.rows)
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// selectedRow returns the row under the cursor, or nil when the tree is empty.
func (m model) selectedRow() *treeRow {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return &m.rows[m.cursor]
	}
	return nil
}

// toggleCollapse folds or unfolds the directory under the cursor and rebuilds.
func (m *model) toggleCollapse() {
	r := m.selectedRow()
	if r == nil || !r.isDir {
		return
	}
	m.collapsed[r.path] = !m.collapsed[r.path]
	m.rebuildTree()
}

func (m model) Init() tea.Cmd { return tickCmd() }

// selectedFile returns the file under the cursor, or nil when the cursor is on a
// folder row (or the tree is empty) — those have no diff to show.
func (m model) selectedFile() *git.FileChange {
	if r := m.selectedRow(); r != nil {
		return r.file
	}
	return nil
}

// expandWindow is how many context lines one expand press reveals, and the gap
// size at or below which a gap collapses to a single "expand all" affordance.
const expandWindow = 20

// loadDiff fetches and parses the diff for the file under the cursor, loads its
// new-side content for context expansion, and derives the display view, resetting
// scroll and the diff cursor.
func (m *model) loadDiff() {
	m.diffOffset = 0
	m.diffCursor = 0
	m.diff = nil
	m.fileLines = nil
	m.gaps = nil
	m.revealed = map[int][2]int{}
	m.viewLines = nil
	m.splitRows = nil
	m.lexer = nil
	m.hlCache = map[string][]span{}
	m.pathLexers = map[string]chroma.Lexer{}
	f := m.selectedFile()
	if f == nil {
		return
	}
	m.lexer = lexerFor(f.Path)
	var raw string
	if m.scopeCommit != nil {
		raw = git.CommitFileDiff(m.repo, m.scopeCommit.SHA, f.Path)
	} else {
		raw = git.FileDiff(m.repo, m.base, f.Path, f.Status)
	}
	if raw == "" {
		m.diff = []diff.Line{{Kind: diff.Meta, Text: "(no textual diff — binary or empty)"}}
	} else {
		m.diff = diff.Parse(raw)
	}
	// Context expansion reveals lines from the working-tree file, which only
	// matches a diff taken against the working tree. A pure deletion has no new
	// side, and a commit-scoped diff is against history (the tree may have moved
	// on since), so neither offers expandable context.
	if m.scopeCommit == nil && f.Status != "D" {
		if fl, err := git.FileContent(m.repo, f.Path); err == nil {
			m.fileLines = fl
			m.gaps = diff.Gaps(m.diff, len(fl))
		}
	}
	m.rebuildView()
}

// lineLexer picks the syntax lexer for one diff line. Single-file views (branch
// and per-commit drill-in) share m.lexer. The commit-history preview (viewLog)
// shows a combined multi-file patch, so each line is highlighted with the lexer
// for its own Path, memoized in pathLexers.
func (m model) lineLexer(l diff.Line) chroma.Lexer {
	if m.mode != viewLog {
		return m.lexer
	}
	if l.Path == "" {
		return nil
	}
	if m.pathLexers != nil {
		if lx, ok := m.pathLexers[l.Path]; ok {
			return lx
		}
	}
	lx := lexerFor(l.Path)
	if m.pathLexers != nil {
		m.pathLexers[l.Path] = lx
	}
	return lx
}

// rebuildView re-derives the display line list (and its split projection) from
// the pristine diff plus the current expansion state, then re-clamps scroll and
// cursor. Called on load, on every expansion, and on a split toggle.
func (m *model) rebuildView() {
	m.viewLines = diff.BuildView(m.diff, m.fileLines, m.gaps, m.revealed, expandWindow)
	m.splitRows = diff.SplitRows(m.viewLines)
	m.lineDigits = lineDigits(m.viewLines)
	m.clampDiffOffset()
	m.clampDiffCursor()
}

// totalDiffRows is the number of scrollable rows in the current view mode.
func (m model) totalDiffRows() int {
	if m.splitView {
		return len(m.splitRows)
	}
	return len(m.viewLines)
}

// lineDigits sizes the line-number gutter from the largest line number, clamped
// to a sane range so a huge file doesn't eat the whole pane.
func lineDigits(lines []diff.Line) int {
	maxN := 0
	for _, l := range lines {
		if l.OldNum > maxN {
			maxN = l.OldNum
		}
		if l.NewNum > maxN {
			maxN = l.NewNum
		}
	}
	d := len(strconv.Itoa(maxN))
	switch {
	case d < 2:
		return 2
	case d > 5:
		return 5
	default:
		return d
	}
}

// diffViewportHeight is the number of diff rows visible in the right pane after
// chrome (header line + divider + footer).
func (m model) diffViewportHeight() int {
	h := m.height - 4
	if h < 1 {
		return 1
	}
	return h
}

// listViewportHeight is the number of file rows visible in the left pane.
func (m model) listViewportHeight() int {
	return m.diffViewportHeight()
}

func (m *model) clampDiffOffset() {
	max := m.totalDiffRows() - m.diffViewportHeight()
	if max < 0 {
		max = 0
	}
	if m.diffOffset > max {
		m.diffOffset = max
	}
	if m.diffOffset < 0 {
		m.diffOffset = 0
	}
}

func (m *model) clampDiffCursor() {
	if m.diffCursor >= m.totalDiffRows() {
		m.diffCursor = m.totalDiffRows() - 1
	}
	if m.diffCursor < 0 {
		m.diffCursor = 0
	}
}

// ensureCursorVisible scrolls the diff pane just enough to keep the cursor row
// inside the viewport.
func (m *model) ensureCursorVisible() {
	if m.diffCursor < m.diffOffset {
		m.diffOffset = m.diffCursor
	}
	if bottom := m.diffOffset + m.diffViewportHeight() - 1; m.diffCursor > bottom {
		m.diffOffset = m.diffCursor - m.diffViewportHeight() + 1
	}
	m.clampDiffOffset()
}

// cursorLine returns the diff line under the cursor in the current view mode, or
// nil when there is none. In split mode it reaches through to the row's Full
// line (the only kind an expand affordance occupies).
func (m model) cursorLine() *diff.Line {
	if m.diffCursor < 0 {
		return nil
	}
	if m.splitView {
		if m.diffCursor < len(m.splitRows) {
			return m.splitRows[m.diffCursor].Full
		}
		return nil
	}
	if m.diffCursor < len(m.viewLines) {
		return &m.viewLines[m.diffCursor]
	}
	return nil
}

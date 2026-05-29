package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diff-master/internal/diff"
	"github.com/trebaud/diff-master/internal/git"
)

// focusPane identifies which of the two panes vim motions (j/k/gg/G) act on.
type focusPane int

const (
	focusFiles focusPane = iota // left: the changed-file list
	focusDiff                   // right: the unified diff
)

// model is the Elm-architecture state for the diff viewer. The left pane lists
// changed files; the right pane shows the selected file's diff. Diff scrolling
// is tracked manually via diffOffset rather than a viewport component, matching
// the rest of the rendering which is hand-laid-out.
type model struct {
	focus    focusPane // pane that j/k/gg/G operate on
	pendingG bool      // first half of the `gg` chord was pressed

	repo      string
	base      string // ref the diff is computed against (merge base of master/HEAD)
	baseName  string // human label for the base branch
	branch    string // current branch
	shortstat string

	files  []git.FileChange
	cursor int // index into files

	// Parsed diff for the selected file, plus the side-by-side projection and
	// the scroll position. lineDigits sizes the line-number gutter.
	diff       []diff.Line
	splitRows  []diff.Row
	splitView  bool // false = unified (GitHub inline), true = side-by-side
	lineDigits int
	diffOffset int

	width  int
	height int

	animFrame int // drives the nyan cat's leg/face wiggle

	showHelp bool
	err      error
}

func newModel(repo, base, baseName, branch string, files []git.FileChange, shortstat string) model {
	m := model{
		repo:      repo,
		base:      base,
		baseName:  baseName,
		branch:    branch,
		shortstat: shortstat,
		files:     files,
	}
	m.loadDiff()
	return m
}

func (m model) Init() tea.Cmd { return tickCmd() }

// selectedFile returns the file under the cursor, or nil when the diff is empty.
func (m model) selectedFile() *git.FileChange {
	if m.cursor >= 0 && m.cursor < len(m.files) {
		return &m.files[m.cursor]
	}
	return nil
}

// loadDiff fetches and parses the diff for the file under the cursor, building
// the side-by-side projection and resetting the scroll position.
func (m *model) loadDiff() {
	m.diffOffset = 0
	m.diff = nil
	m.splitRows = nil
	f := m.selectedFile()
	if f == nil {
		return
	}
	raw := git.FileDiff(m.repo, m.base, f.Path, f.Status)
	if raw == "" {
		m.diff = []diff.Line{{Kind: diff.Meta, Text: "(no textual diff — binary or empty)"}}
	} else {
		m.diff = diff.Parse(raw)
	}
	m.splitRows = diff.SplitRows(m.diff)
	m.lineDigits = lineDigits(m.diff)
}

// totalDiffRows is the number of scrollable rows in the current view mode.
func (m model) totalDiffRows() int {
	if m.splitView {
		return len(m.splitRows)
	}
	return len(m.diff)
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

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// branchpick.go is the branch switcher (`b`, history view only): a fuzzy
// picker of local branches that checks the selected one out. The switch runs
// off the UI thread and lands as a branchSwitchMsg, whose handler refreshes
// the model immediately so the commit history repaints for the new branch
// without waiting on the background poll. Git itself is the dirty-tree guard:
// a switch it refuses surfaces its reason in the footer flash.

// openBranchPicker opens the switcher. The branch list is read fresh on each
// open — a cheap for-each-ref — minus the checked-out branch, since switching
// to it is a no-op.
func (m *model) openBranchPicker() {
	m.branchPickList = nil
	for _, b := range git.LocalBranches(m.repo) {
		if b.Name != m.branch {
			m.branchPickList = append(m.branchPickList, b)
		}
	}
	m.branchPickActive = true
	m.branchPickInput = ""
	m.branchPickSel = 0
}

// branchPickMatches ranks the branch names against the picker query, reusing
// the file finder's fuzzy matcher.
func (m model) branchPickMatches() []fileMatch {
	names := make([]string, len(m.branchPickList))
	for i, b := range m.branchPickList {
		names[i] = b.Name
	}
	return fuzzyFilter(names, m.branchPickInput)
}

// branchByName resolves a match's name back to its Branch metadata; nil when
// it's gone (it can't be, but the picker never trusts an index).
func (m model) branchByName(name string) *git.Branch {
	for i := range m.branchPickList {
		if m.branchPickList[i].Name == name {
			return &m.branchPickList[i]
		}
	}
	return nil
}

// branchSwitchMsg carries the result of a background `git switch` back to the
// UI thread; branch is the name the switch targeted.
type branchSwitchMsg struct {
	branch string
	err    error
}

// switchBranchCmd runs the checkout off the UI thread — a switch that touches
// many files can take a moment, and the render loop shouldn't stall on it.
func switchBranchCmd(repo, name string) tea.Cmd {
	return func() tea.Msg {
		return branchSwitchMsg{branch: name, err: git.Switch(repo, name)}
	}
}

// branchPickRows is the most result rows the picker shows at once; a short
// terminal gets fewer (see branchPickRowCount).
const branchPickRows = 16

// branchPickRowCount is how many result rows the box reserves. It's sized to
// the repo's branch count — not the filtered matches, so the box holds its
// height while a query narrows the list instead of resizing on every keystroke
// — capped by branchPickRows and by what the terminal can hold. The box's
// chrome is 8 rows (a 1-col border and 1-row padding top and bottom, plus the
// four header lines), and floatOverlay clips a box taller than the screen —
// which would cut off the selected row rather than scroll to it.
func (m model) branchPickRowCount() int {
	rows := len(m.branchPickList)
	if fit := m.height - 8; rows > fit {
		rows = fit
	}
	if rows > branchPickRows {
		rows = branchPickRows
	}
	if rows < 3 {
		rows = 3
	}
	return rows
}

// branchPickBox builds the switcher as a floating window: a title, a prompt
// line, a match count, then up to branchPickRows branches windowed around the
// selection. Each row shows the branch name (fuzzy hits bolded), its remote
// when it tracks one, and the tip commit's author · date flush-right.
//
// Every fragment — the padding included — paints the overlay background
// itself: floatingBox's block background stops at the first embedded ANSI
// reset, which is what left darker patches around the input.
func (m model) branchPickBox() string {
	bg := lipgloss.NewStyle().Background(colOverlayBg)
	sel := selectedStyle.Background(colOverlayBg)
	mut := mutedStyle.Background(colOverlayBg)
	head := headingStyle.Background(colOverlayBg)
	pad := func(s string, w int) string {
		s = lipgloss.NewStyle().MaxWidth(w).Render(s)
		if gap := w - lipgloss.Width(s); gap > 0 {
			s += bg.Render(strings.Repeat(" ", gap))
		}
		return s
	}

	matches := m.branchPickMatches()
	w := m.width - 6
	if w > 88 {
		w = 88
	}
	if w < 20 {
		w = 20
	}
	rows := m.branchPickRowCount()

	selIdx := m.branchPickSel
	if selIdx >= len(matches) {
		selIdx = max(0, len(matches)-1)
	}
	offset := 0
	if selIdx >= rows {
		offset = selIdx - rows + 1
	}
	end := min(offset+rows, len(matches))

	lines := []string{
		pad(head.Render("switch branch"), w),
		pad(sel.Render("❯ ")+bg.Render(m.branchPickInput)+sel.Render("▌"), w),
		pad(mut.Render(pluralMatches(len(matches))), w),
		pad("", w),
	}
	if len(matches) == 0 {
		lines = append(lines, pad(mut.Render("  no branches match"), w))
	}
	shown := 0
	for i := offset; i < end; i++ {
		lines = append(lines, m.branchPickRow(matches[i], i == selIdx, w))
		shown++
	}
	// Pad to a stable height so the box doesn't jump as the result count changes.
	for ; shown < rows; shown++ {
		lines = append(lines, pad("", w))
	}
	return floatingBox(strings.Join(lines, "\n"))
}

// branchPickRow renders one branch: the name (fuzzy hits bolded), a muted
// remote tag when the branch tracks one (e.g. "origin"), and the tip commit's
// "author · date" flush-right. The selected row is a continuous highlight bar.
func (m model) branchPickRow(bm fileMatch, selected bool, width int) string {
	remote, right := "", ""
	if b := m.branchByName(bm.path); b != nil {
		if r := b.Remote(); r != "" {
			remote = " ⇡" + r
		}
		var parts []string
		if b.Author != "" {
			parts = append(parts, truncateText(b.Author, 20))
		}
		if b.Date != "" {
			parts = append(parts, b.Date)
		}
		right = strings.Join(parts, " · ")
	}
	// The name gets whatever width the tag and metadata leave; a narrow box
	// drops the metadata, then the tag, rather than the name.
	avail := width - 2 - lipgloss.Width(remote) - lipgloss.Width(right) - 2
	if avail < 12 {
		right = ""
		avail = width - 2 - lipgloss.Width(remote)
	}
	if avail < 8 {
		remote = ""
		avail = width - 2
	}
	if selected {
		// Branch names share their distinguishing part at the *front*
		// (feature/thing-a vs feature/thing-b), unlike file paths where the
		// basename matters — so a long one is cropped on the right.
		name := truncateText(bm.path, avail)
		gap := width - 2 - lipgloss.Width(name) - lipgloss.Width(remote) - lipgloss.Width(right)
		if gap < 0 {
			gap = 0
		}
		return selectedRowStyle.Width(width).Render("  " + name + remote + strings.Repeat(" ", gap) + right)
	}
	// styleFuzzyOverlay truncates to avail itself (keeping the hit offsets
	// aligned to the original runes), so its display width is min(len, avail).
	nameW := len([]rune(bm.path))
	if nameW > avail {
		nameW = avail
	}
	gap := width - 2 - nameW - lipgloss.Width(remote) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	bg := lipgloss.NewStyle().Background(colOverlayBg)
	mut := mutedStyle.Background(colOverlayBg)
	row := bg.Render("  ") + styleFuzzyOverlay(bm.path, bm.hit, avail) + mut.Render(remote) +
		bg.Render(strings.Repeat(" ", gap)) + mut.Render(right)
	return lipgloss.NewStyle().MaxWidth(width).Render(row)
}

// styleFuzzyOverlay renders a branch name bolding the rune offsets in hit,
// cropping the right side (keeping the head visible) with a trailing ellipsis
// when it's too long — branch names differ at the front, so that's the end to
// keep. Every fragment carries the overlay background (see the background note
// on branchPickBox). Hit offsets past the crop are simply not reached.
func styleFuzzyOverlay(name string, hit []int, max int) string {
	if max < 2 {
		max = 2
	}
	bg := lipgloss.NewStyle().Background(colOverlayBg)
	mut := mutedStyle.Background(colOverlayBg)
	hitStyle := lipgloss.NewStyle().Foreground(colSelect).Bold(true).Background(colOverlayBg)
	hitSet := make(map[int]bool, len(hit))
	for _, h := range hit {
		hitSet[h] = true
	}
	runes := []rune(name)
	end := len(runes)
	cropped := false
	if end > max {
		end = max - 1 // leave a column for the ellipsis
		cropped = true
	}
	var b strings.Builder
	for i := 0; i < end; i++ {
		if hitSet[i] {
			b.WriteString(hitStyle.Render(string(runes[i])))
		} else {
			b.WriteString(bg.Render(string(runes[i])))
		}
	}
	if cropped {
		b.WriteString(mut.Render("…"))
	}
	return b.String()
}

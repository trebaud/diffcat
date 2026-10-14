package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// selection.go is mouse text selection. Mouse reporting is on (for the wheel),
// so the terminal's own selection would need a modifier and would run across
// both panes. Instead a left-drag selects inside the pane it started in — the
// file tree or the diff — clamped to that pane's columns and the body rows, and
// the text is copied to the clipboard on release. The highlight stays until the
// next click, key, or wheel.

// mouseSel is an in-progress or finished drag. Coordinates are screen cells;
// the pane rectangle is fixed by where the drag started.
type mouseSel struct {
	left, right, top, bottom int // pane bounds, inclusive
	ax, ay                   int // anchor (press)
	cx, cy                   int // current end
	dragging                 bool
}

// selectionPane returns the bounds of the pane under (x, y), or ok=false over
// the header, footer, or the divider.
func (m model) selectionPane(x, y int) (left, right, top, bottom int, ok bool) {
	top, bottom = 1, m.height-2
	if y < top || y > bottom {
		return 0, 0, 0, 0, false
	}
	lw := m.sidebarWidth()
	full := m.mode == viewOverview || m.mode == viewAuthorDetail ||
		(m.mode == viewLog && !m.logDiffOpen) || lw == 0
	switch {
	case full:
		return 0, m.width - 1, top, bottom, true
	case x < lw:
		return 0, lw - 1, top, bottom, true
	case x > lw:
		return lw + 1, m.width - 1, top, bottom, true
	}
	return 0, 0, 0, 0, false
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// ordered returns the selection endpoints in reading order.
func (s *mouseSel) ordered() (x0, y0, x1, y1 int) {
	if s.ay < s.cy || (s.ay == s.cy && s.ax <= s.cx) {
		return s.ax, s.ay, s.cx, s.cy
	}
	return s.cx, s.cy, s.ax, s.ay
}

// span returns the selected column range on row y, like a terminal's linear
// selection but bounded by the pane: the first row runs from the start to the
// pane edge, middle rows are whole, the last row ends at the end point.
func (s *mouseSel) span(y int) (from, to int, ok bool) {
	x0, y0, x1, y1 := s.ordered()
	if y < y0 || y > y1 {
		return 0, 0, false
	}
	from, to = s.left, s.right
	if y == y0 {
		from = x0
	}
	if y == y1 {
		to = x1
	}
	return from, to, true
}

func (m model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	m.sel = nil
	if msg.Button != tea.MouseLeft || m.loading || m.anyOverlay() {
		return m, nil
	}
	l, r, t, b, ok := m.selectionPane(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	m.sel = &mouseSel{left: l, right: r, top: t, bottom: b, ax: msg.X, ay: msg.Y, cx: msg.X, cy: msg.Y, dragging: true}
	return m, nil
}

func (m model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.sel == nil || !m.sel.dragging {
		return m, nil
	}
	s := *m.sel
	s.cx = clampInt(msg.X, s.left, s.right)
	s.cy = clampInt(msg.Y, s.top, s.bottom)
	m.sel = &s
	return m, nil
}

func (m model) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if m.sel == nil || !m.sel.dragging {
		return m, nil
	}
	s := *m.sel
	s.cx = clampInt(msg.X, s.left, s.right)
	s.cy = clampInt(msg.Y, s.top, s.bottom)
	s.dragging = false
	if s.cx == s.ax && s.cy == s.ay {
		m.sel = nil // a plain click selects nothing
		return m, nil
	}
	m.sel = &s
	text := m.selectedText()
	if text == "" {
		return m, nil
	}
	n := strings.Count(text, "\n") + 1
	if n == 1 {
		m.flash = "copied selection"
	} else {
		m.flash = "copied " + strconv.Itoa(n) + " lines"
	}
	return m, tea.SetClipboard(text)
}

// screenCanvas renders the base screen (no overlays) onto a cell canvas.
func (m model) screenCanvas(screen string) *lipgloss.Canvas {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	return canvas
}

// selectedText reads the selected cells back off the rendered screen, one line
// per row with trailing padding trimmed.
func (m model) selectedText() string {
	canvas := m.screenCanvas(m.baseScreen())
	_, y0, _, y1 := m.sel.ordered()
	var lines []string
	for y := y0; y <= y1; y++ {
		from, to, _ := m.sel.span(y)
		var b strings.Builder
		for x := from; x <= to; x++ {
			if c := canvas.CellAt(x, y); c != nil {
				b.WriteString(c.Content)
			}
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	return strings.Join(lines, "\n")
}

// drawSelection reverses the selected cells of screen.
func (m model) drawSelection(screen string) string {
	canvas := m.screenCanvas(screen)
	_, y0, _, y1 := m.sel.ordered()
	for y := y0; y <= y1; y++ {
		from, to, _ := m.sel.span(y)
		for x := from; x <= to; x++ {
			if c := canvas.CellAt(x, y); c != nil {
				c.Style.Attrs ^= uv.AttrReverse
			}
		}
	}
	return canvas.Render()
}

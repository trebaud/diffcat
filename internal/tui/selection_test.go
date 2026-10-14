package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A drag that starts in the diff pane and runs off to the left stays clamped to
// the diff pane: no sidebar text leaks into the copied selection.
func TestMouseSelectionStaysInPane(t *testing.T) {
	m := sampleModel()
	m.width, m.height = 120, 30
	lw := m.sidebarWidth()
	if lw == 0 {
		t.Fatal("sample model should show the sidebar")
	}
	var tm tea.Model = m
	tm, _ = tm.(model).handleMouseClick(tea.MouseClickMsg{X: m.width - 1, Y: 2, Button: tea.MouseLeft})
	tm, _ = tm.(model).handleMouseMotion(tea.MouseMotionMsg{X: 0, Y: 5, Button: tea.MouseLeft})
	tm, cmd := tm.(model).handleMouseRelease(tea.MouseReleaseMsg{X: 0, Y: 5, Button: tea.MouseLeft})
	got := tm.(model)
	if cmd == nil {
		t.Fatal("expected a clipboard command on release")
	}
	if got.sel.left != lw+1 || got.sel.cx != lw+1 {
		t.Fatalf("selection escaped the diff pane: %+v (sidebar %d)", *got.sel, lw)
	}
	if n := strings.Count(got.selectedText(), "\n"); n != 3 {
		t.Fatalf("want 4 lines, got %d", n+1)
	}
	if !strings.Contains(got.render(), "\x1b[") {
		t.Fatal("render lost styling")
	}

	// A plain click clears the selection.
	tm, _ = got.handleMouseClick(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	tm, _ = tm.(model).handleMouseRelease(tea.MouseReleaseMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	if tm.(model).sel != nil {
		t.Fatal("click without drag should leave no selection")
	}
}

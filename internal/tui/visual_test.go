package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
)

// writeCommit overwrites a.txt with body and commits it.
func writeCommit(t *testing.T, repo, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte(body+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "a.txt")
	gitIn(t, repo, "commit", "-m", msg)
}

// changed collects the text of the added and deleted lines of a parsed diff.
func changed(lines []diff.Line) (added, deleted []string) {
	for _, l := range lines {
		switch l.Kind {
		case diff.Add:
			added = append(added, l.Text)
		case diff.Del:
			deleted = append(deleted, l.Text)
		}
	}
	return added, deleted
}

// TestVisualRangeShowsNetDiff selects the two newest of three commits that each
// rewrite the same line. The range must show its net change — the line as it
// stood before the range against the newest commit's version — so the middle
// commit's intermediate text never appears (the latest commit wins).
func TestVisualRangeShowsNetDiff(t *testing.T) {
	repo := initSyncRepo(t)
	writeCommit(t, repo, "one", "c1")
	writeCommit(t, repo, "two", "c2")
	writeCommit(t, repo, "three", "c3")
	m := newSyncModel(t, repo)

	m.toggleVisual()
	m.moveCommitCursor(1) // rows 0..1: c3 and c2
	r := m.selectionRange()
	if r == nil || r.commits != 2 || r.working {
		t.Fatalf("selectionRange = %+v, want 2 commits without working tree", r)
	}

	m.openLogDiff()
	add, del := changed(m.diff)
	if strings.Join(add, ",") != "three" || strings.Join(del, ",") != "one" {
		t.Fatalf("preview +%v -%v, want +[three] -[one]", add, del)
	}

	m.enterRange()
	if m.mode != viewCommit || m.scopeRange == nil {
		t.Fatalf("enterRange: mode=%v scopeRange=%v", m.mode, m.scopeRange)
	}
	if len(m.files) != 1 || m.files[0].Path != "a.txt" {
		t.Fatalf("range files = %+v, want [a.txt]", m.files)
	}
	add, del = changed(m.diff)
	if strings.Join(add, ",") != "three" || strings.Join(del, ",") != "one" {
		t.Fatalf("drill-in +%v -%v, want +[three] -[one]", add, del)
	}

	// Esc back keeps the selection marked; a second toggle clears it.
	m.exitCommit()
	if m.mode != viewLog || !m.logVisual {
		t.Fatalf("after exitCommit: mode=%v logVisual=%v", m.mode, m.logVisual)
	}
	m.toggleVisual()
	if m.logVisual || m.selectionRange() != nil {
		t.Fatal("toggleVisual did not clear the selection")
	}
}

// TestVisualRangeIncludesWorkingTree selects the working-tree row plus the newest
// commit: the range then ends at the working tree rather than a commit.
func TestVisualRangeIncludesWorkingTree(t *testing.T) {
	repo := initSyncRepo(t)
	writeCommit(t, repo, "one", "c1")
	writeCommit(t, repo, "two", "c2")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := newSyncModel(t, repo)
	if !m.logWorking {
		t.Fatal("expected a working-tree row")
	}

	m.toggleVisual()
	m.moveCommitCursor(1) // working tree + c2
	r := m.selectionRange()
	if r == nil || !r.working || r.commits != 1 {
		t.Fatalf("selectionRange = %+v, want working tree + 1 commit", r)
	}
	m.openLogDiff()
	add, del := changed(m.diff)
	if strings.Join(add, ",") != "dirty" || strings.Join(del, ",") != "one" {
		t.Fatalf("preview +%v -%v, want +[dirty] -[one]", add, del)
	}
}

// TestVisualSingleRowFallsBack: a one-row selection is just the cursor, so the
// regular single-commit preview applies.
func TestVisualSingleRowFallsBack(t *testing.T) {
	repo := initSyncRepo(t)
	writeCommit(t, repo, "one", "c1")
	m := newSyncModel(t, repo)
	m.toggleVisual()
	if m.selectionRange() != nil {
		t.Fatal("single-row selection should not resolve to a range")
	}
	if !m.inVisual(0) || m.inVisual(1) {
		t.Fatal("inVisual bounds wrong for a single-row selection")
	}
}

// TestVisualKeysAndRender drives the real key path (v, j, Enter, Esc) and renders
// each state, so the highlight and titles are exercised end to end.
func TestVisualKeysAndRender(t *testing.T) {
	repo := initSyncRepo(t)
	writeCommit(t, repo, "one", "c1")
	writeCommit(t, repo, "two", "c2")
	m := newSyncModel(t, repo)
	m.width, m.height = 120, 30

	press := func(code rune, text string) {
		t.Helper()
		next, _ := m.handleKey(tea.KeyPressMsg{Code: code, Text: text})
		m = next.(model)
		_ = m.View()
	}
	press('v', "v")
	press('j', "j")
	if !m.logVisual || m.selectionRange() == nil {
		t.Fatal("v then j should select a two-commit range")
	}
	if !strings.Contains(m.headerView(), "VISUAL") {
		t.Error("header should flag visual mode")
	}
	press(tea.KeyEnter, "")
	if m.mode != viewCommit || m.scopeRange == nil {
		t.Fatalf("Enter on a range: mode=%v scopeRange=%v", m.mode, m.scopeRange)
	}
	if !strings.Contains(m.headerView(), "2 commits") {
		t.Errorf("range header = %q", m.headerView())
	}
	press(tea.KeyEscape, "") // back to the list, selection kept
	press(tea.KeyEscape, "") // leave visual
	if m.mode != viewLog || m.logVisual {
		t.Fatalf("after esc esc: mode=%v logVisual=%v", m.mode, m.logVisual)
	}
}

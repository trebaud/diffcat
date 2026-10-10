package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// TestMain points the config and state files at a scratch directory, so tests
// that toggle a persisted preference never touch the developer's real ones.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "diffcat-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir+"/config")
	os.Setenv("XDG_STATE_HOME", dir+"/state")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// press feeds one key through the real key path.
func press(t *testing.T, m model, key string) model {
	t.Helper()
	var msg tea.KeyPressMsg
	switch key {
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		r := []rune(key)
		msg = tea.KeyPressMsg{Code: r[0], Text: key}
	}
	next, _ := m.handleKey(msg)
	return next.(model)
}

func pressAll(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		m = press(t, m, k)
	}
	return m
}

// The footer keeps whole hints that fit, always leading with `? help`, at every
// width and in every mode.
func TestFooterFitsAndLeadsWithHelp(t *testing.T) {
	overview := sampleModel()
	overview.mode = viewOverview
	for _, m := range []model{sampleModel(), logSampleModel(), commitSampleModel(), overview} {
		for _, w := range []int{60, 80, 120, 200} {
			m.width = w
			f := m.footerView()
			if got := lipgloss.Width(f); got > w {
				t.Errorf("mode=%d width %d: footer is %d wide", m.mode, w, got)
			}
			if !strings.HasPrefix(f, mutedStyle.Render("? help")) {
				t.Errorf("mode=%d width %d: footer should lead with ? help: %q", m.mode, w, f)
			}
		}
	}
}

// Help scrolls with j/k when it overflows the screen, ignores other keys, and
// closes on Esc.
func TestHelpScrollsAndClosesExplicitly(t *testing.T) {
	m := sampleModel()
	m.width, m.height = 100, 20
	m = press(t, m, "?")
	if !m.showHelp {
		t.Fatal("? should open help")
	}
	m = pressAll(t, m, "j", "j")
	if m.helpScroll != 2 {
		t.Errorf("helpScroll = %d after jj, want 2", m.helpScroll)
	}
	m = press(t, m, "x")
	if !m.showHelp {
		t.Error("an unbound key should not close help")
	}
	m = press(t, m, "G")
	if want := len(m.helpLines()) - m.helpBodyRows(); m.helpScroll != want {
		t.Errorf("G: helpScroll = %d, want %d", m.helpScroll, want)
	}
	m = press(t, m, "esc")
	if m.showHelp {
		t.Error("esc should close help")
	}
}

// The current view's section leads the help body.
func TestHelpLeadsWithCurrentMode(t *testing.T) {
	m := logSampleModel()
	if first := m.helpLines()[0]; !strings.Contains(first, "history") {
		t.Errorf("history help should lead with the history section, got %q", first)
	}
	m = sampleModel()
	if first := m.helpLines()[0]; !strings.Contains(first, "files & diff") {
		t.Errorf("branch help should lead with the files section, got %q", first)
	}
}

// ]c steps to the start of each change block; past the last it explains why it
// didn't move. [c steps back.
func TestJumpChange(t *testing.T) {
	m := sampleModel()
	m.mode = viewBranch
	m.focus = focusDiff
	m.width, m.height = 120, 40
	m = pressAll(t, m, "]", "c")
	if !m.changeStart(m.diffCursor) || m.viewLines[m.diffCursor].Kind != diff.Del {
		t.Fatalf("]c should land on the first removed line, at %d (%v)", m.diffCursor, m.viewLines[m.diffCursor].Kind)
	}
	first := m.diffCursor
	m = pressAll(t, m, "]", "c")
	if m.diffCursor != first || m.flash == "" {
		t.Errorf("]c past the only change block should stay and explain; cursor %d flash %q", m.diffCursor, m.flash)
	}
	m = pressAll(t, m, "G", "[", "c")
	if m.diffCursor != first {
		t.Errorf("[c from the bottom should return to the block start %d, got %d", first, m.diffCursor)
	}
}

// A count prefix repeats j, and {count}G jumps to that line of the diff.
func TestCountPrefix(t *testing.T) {
	m := sampleModel()
	m.focus = focusDiff
	m.width, m.height = 120, 40
	m = pressAll(t, m, "3", "j")
	if m.diffCursor != 3 {
		t.Errorf("3j: cursor %d, want 3", m.diffCursor)
	}
	if m.count != "" {
		t.Errorf("count should be consumed, got %q", m.count)
	}
	m = pressAll(t, m, "1", "2", "G")
	if l := m.rowLine(m.diffCursor); l == nil || l.NewNum != 12 {
		t.Errorf("12G should land on new line 12, got %+v", l)
	}
}

// J / K step between files in the tree while keeping the diff focused.
func TestJumpFileKeepsFocus(t *testing.T) {
	m := sampleModel()
	m.mode = viewBranch
	m.focus = focusDiff
	before := m.selectedFile().Path
	m = press(t, m, "J")
	if f := m.selectedFile(); f == nil || f.Path == before {
		t.Fatalf("J should move to the next file, still on %q", before)
	}
	if m.focus != focusDiff {
		t.Error("J should keep the diff pane focused")
	}
	m = press(t, m, "K")
	if f := m.selectedFile(); f == nil || f.Path != before {
		t.Errorf("K should come back to %q", before)
	}
}

// Under soft-wrap the cursor row is always wholly on screen, and the rows on
// screen never exceed the viewport.
func TestWrapKeepsCursorVisible(t *testing.T) {
	m := sampleModel()
	m.focus = focusDiff
	m.wrap = true
	m.width, m.height = 70, 14
	for i := 0; i < m.totalDiffRows(); i++ {
		m.diffCursor = i
		m.ensureCursorVisible()
		if m.diffCursor < m.diffOffset || m.rowsHeight(m.diffOffset, m.diffCursor) > m.diffViewportHeight() && m.diffOffset != m.diffCursor {
			t.Fatalf("cursor %d not wholly visible from offset %d", i, m.diffOffset)
		}
	}
	if got := len(m.diffWindow(m.diffContentWidth(), m.diffViewportHeight())); got > m.diffViewportHeight() {
		t.Errorf("wrapped window has %d rows, viewport %d", got, m.diffViewportHeight())
	}
}

// Esc first clears a committed search, and only then steps back.
func TestEscClearsSearchFirst(t *testing.T) {
	m := commitSampleModel()
	m.searchQuery = "line"
	m.recomputeSearch()
	m = press(t, m, "esc")
	if m.searchQuery != "" || m.mode != viewCommit {
		t.Errorf("first esc should only clear the search: query %q mode %d", m.searchQuery, m.mode)
	}
}

// Keys that do nothing in the current view say so instead of failing silently.
func TestInertKeysExplain(t *testing.T) {
	cases := []struct {
		m   model
		key string
	}{
		{logSampleModel(), "s"},
		{sampleModel(), "b"},
		{sampleModel(), "v"},
		{sampleModel(), "n"},
		{logSampleModel(), "<"},
	}
	for _, c := range cases {
		if got := press(t, c.m, c.key); got.flash == "" {
			t.Errorf("mode=%d key %q: no explanation", c.m.mode, c.key)
		}
	}
}

// `y` copies the commit SHA in history, the path from the tree, and path:line
// from the diff pane.
func TestCopyTarget(t *testing.T) {
	m := logSampleModel()
	if got := m.copyTarget(); got != "aaa111full" {
		t.Errorf("history copy = %q, want the full SHA", got)
	}
	m = sampleModel()
	m.mode = viewBranch
	// The sample diff's lines belong to view.go; put that file under the cursor.
	f := git.FileChange{Path: "view.go", Status: "M"}
	m.rows, m.cursor = []treeRow{{file: &f, path: f.Path}}, 0
	path := m.selectedFile().Path
	if got := m.copyTarget(); got != path {
		t.Errorf("tree copy = %q, want %q", got, path)
	}
	m.focus = focusDiff
	m.diffCursor = len(m.viewLines) - 1
	if got := m.copyTarget(); !strings.HasPrefix(got, path+":") {
		t.Errorf("diff copy = %q, want %s:<line>", got, path)
	}
}

// Saving preferences keeps keys the TUI doesn't manage, like the editor.
func TestPersistPrefsKeepsEditor(t *testing.T) {
	if err := saveConfig(userConfig{Editor: "nvim"}); err != nil {
		t.Fatal(err)
	}
	m := sampleModel()
	m.splitView, m.sidebar = true, sidebarWide
	m.persistPrefs()
	cfg := loadConfig()
	if cfg.Editor != "nvim" || !cfg.Split || cfg.Sidebar != "wide" {
		t.Errorf("config after save = %+v", cfg)
	}
	if r := resolveOptions(Options{}, cfg); !r.split || r.sidebar != sidebarWide {
		t.Errorf("layout prefs not restored: split=%v sidebar=%v", r.split, r.sidebar)
	}
}

// Esc from the branch diff returns to the history row `D` was pressed on, with
// its diff pane still open.
func TestBranchDiffEscReturnsToOrigin(t *testing.T) {
	repo := initSyncRepo(t)
	gitIn(t, repo, "switch", "-c", "feature")
	writeCommit(t, repo, "one", "c1")
	writeCommit(t, repo, "two", "c2")
	m := newSyncModel(t, repo)
	m.width, m.height = 120, 30
	m = pressAll(t, m, "j", "l")
	sha := m.selectedCommit().SHA
	m = press(t, m, "D")
	if m.mode != viewBranch {
		t.Fatalf("D should open the branch diff, mode=%d flash=%q", m.mode, m.flash)
	}
	m = press(t, m, "esc")
	if m.mode != viewLog || m.selectedCommit() == nil || m.selectedCommit().SHA != sha || !m.logDiffOpen {
		t.Errorf("esc should return to commit %s with the diff open; mode=%d open=%v", sha, m.mode, m.logDiffOpen)
	}
}

// Ignoring whitespace drops whitespace-only edits from the patch.
func TestIgnoreWhitespaceDiff(t *testing.T) {
	repo := initSyncRepo(t)
	writeCommit(t, repo, "body", "base")
	if err := os.WriteFile(repo+"/a.txt", []byte("  body  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { git.IgnoreWhitespace = false }()
	if git.FileDiff(repo, "HEAD", "a.txt", "M") == "" {
		t.Fatal("expected a whitespace diff")
	}
	git.IgnoreWhitespace = true
	if d := git.FileDiff(repo, "HEAD", "a.txt", "M"); d != "" {
		t.Errorf("with -w the whitespace-only change should vanish, got %q", d)
	}
}

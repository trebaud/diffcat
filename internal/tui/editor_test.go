package tui

import (
	"reflect"
	"testing"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

func TestResolveEditorPrecedence(t *testing.T) {
	cfg := userConfig{Editor: "cfg-editor"}
	t.Setenv("DIFFCAT_EDITOR", "")
	t.Setenv("VISUAL", "visual")
	t.Setenv("EDITOR", "editor")

	if got := resolveEditor("flag", cfg); got != "flag" {
		t.Errorf("flag should win, got %q", got)
	}

	t.Setenv("DIFFCAT_EDITOR", "  env-editor  ")
	if got := resolveEditor("", cfg); got != "env-editor" {
		t.Errorf("DIFFCAT_EDITOR should win over config and be trimmed, got %q", got)
	}

	t.Setenv("DIFFCAT_EDITOR", "")
	if got := resolveEditor("", cfg); got != "cfg-editor" {
		t.Errorf("config should win over $VISUAL/$EDITOR, got %q", got)
	}

	if got := resolveEditor("", userConfig{}); got != "visual" {
		t.Errorf("$VISUAL should win over $EDITOR, got %q", got)
	}

	t.Setenv("VISUAL", "")
	if got := resolveEditor("", userConfig{}); got != "editor" {
		t.Errorf("$EDITOR is the last resort, got %q", got)
	}

	t.Setenv("EDITOR", "")
	if got := resolveEditor("", userConfig{}); got != defaultEditor {
		t.Errorf("nothing configured should fall back to %q, got %q", defaultEditor, got)
	}
}

func TestEditorArgv(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		line int
		want []string
	}{
		{"vim family takes +N before the path", "nvim", 42, []string{"nvim", "+42", "/r/a.go"}},
		{"absolute path and extension still match", "/usr/bin/vim.exe", 7, []string{"/usr/bin/vim.exe", "+7", "/r/a.go"}},
		{"vscode family uses --goto", "code", 9, []string{"code", "--goto", "/r/a.go:9"}},
		{"extra flags are preserved", "code -w", 9, []string{"code", "-w", "--goto", "/r/a.go:9"}},
		{"colon editors glue the line to the path", "hx", 3, []string{"hx", "/r/a.go:3"}},
		{"unknown editors just get the path", "myedit", 3, []string{"myedit", "/r/a.go"}},
		{"no line means no jump", "nvim", 0, []string{"nvim", "/r/a.go"}},
	}
	for _, c := range cases {
		if got := editorArgv(c.cmd, "/r/a.go", c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: editorArgv(%q, %d) = %v, want %v", c.name, c.cmd, c.line, got, c.want)
		}
	}
	if got := editorArgv("   ", "/r/a.go", 1); got != nil {
		t.Errorf("blank editor should yield no argv, got %v", got)
	}
}

// The cursor often rests on a row with no new-side line (a removal, a hunk
// header, an expand affordance); the editor should still land near it rather
// than at the top of the file.
func TestEditorLineWalksBackToANewSideLine(t *testing.T) {
	m := model{focus: focusDiff}
	m.viewLines = []diff.Line{
		{Kind: diff.Hunk, Text: "@@ -1,3 +1,3 @@"},
		{Kind: diff.Context, Text: "a", OldNum: 1, NewNum: 1},
		{Kind: diff.Add, Text: "b", NewNum: 2},
		{Kind: diff.Del, Text: "c", OldNum: 2},
		{Kind: diff.Expand, Hidden: 4},
	}

	for cursor, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 2, 4: 2} {
		m.diffCursor = cursor
		if got := m.editorLine(""); got != want {
			t.Errorf("cursor %d: editorLine() = %d, want %d", cursor, got, want)
		}
	}

	// With the file list focused there is no diff cursor to honor.
	m.diffCursor = 2
	m.focus = focusFiles
	if got := m.editorLine(""); got != 0 {
		t.Errorf("file-list focus should not jump, got %d", got)
	}
}

func TestEditorLineSplitViewReadsTheNewSide(t *testing.T) {
	del := diff.Line{Kind: diff.Del, Text: "old", OldNum: 5}
	add := diff.Line{Kind: diff.Add, Text: "new", NewNum: 6}
	hunk := diff.Line{Kind: diff.Hunk, Text: "@@"}
	m := model{focus: focusDiff, splitView: true}
	m.splitRows = []diff.Row{
		{Full: &hunk},
		{Left: &del, Right: &add},
		{Left: &del}, // unbalanced removal: no right side to read
	}

	m.diffCursor = 1
	if got := m.editorLine(""); got != 6 {
		t.Errorf("paired row should use the added side, got %d", got)
	}
	m.diffCursor = 2
	if got := m.editorLine(""); got != 6 {
		t.Errorf("unbalanced removal should fall back to the row above, got %d", got)
	}
}

// In the history view the diff pane is a whole-commit patch, so the file comes
// from the line under the cursor rather than a tree selection — and only when
// the pane is actually open.
func TestEditorLineStopsAtTheFileBoundary(t *testing.T) {
	// A combined patch (what the history view shows) with two file sections. The
	// walk back for a new-side line must not cross out of the file it is in.
	m := model{mode: viewLog, logDiffOpen: true, focus: focusDiff}
	m.viewLines = []diff.Line{
		{Kind: diff.Meta, Text: "diff --git a/first.go b/first.go"},
		{Kind: diff.Meta, Text: "--- a/first.go", Path: "first.go"},
		{Kind: diff.Meta, Text: "+++ b/first.go", Path: "first.go"},
		{Kind: diff.Hunk, Text: "@@ -240,1 +240,1 @@", Path: "first.go"},
		{Kind: diff.Add, Text: "x", NewNum: 240, Path: "first.go"},
		{Kind: diff.Meta, Text: "diff --git a/second.go b/second.go"},
		{Kind: diff.Meta, Text: "--- a/second.go", Path: "second.go"},
		{Kind: diff.Meta, Text: "+++ b/second.go", Path: "second.go"},
		{Kind: diff.Hunk, Text: "@@ -1,1 +1,1 @@", Path: "second.go"},
		{Kind: diff.Add, Text: "y", NewNum: 1, Path: "second.go"},
	}

	// On second.go's headers and hunk line there is no line of its own yet: open
	// the file at the top rather than at first.go's line 240.
	for _, cursor := range []int{6, 7, 8} {
		m.diffCursor = cursor
		path, line := m.editorTarget()
		if path != "second.go" || line != 0 {
			t.Errorf("cursor %d: editorTarget() = %q:%d, want second.go:0", cursor, path, line)
		}
	}

	// Within second.go the line is its own.
	m.diffCursor = 9
	if path, line := m.editorTarget(); path != "second.go" || line != 1 {
		t.Errorf("editorTarget() = %q:%d, want second.go:1", path, line)
	}

	// first.go still resolves against its own section.
	m.diffCursor = 4
	if path, line := m.editorTarget(); path != "first.go" || line != 240 {
		t.Errorf("editorTarget() = %q:%d, want first.go:240", path, line)
	}
}

// Context revealed by an expand press is synthesized rather than parsed, so it
// carries no path — it must still count as a line in the surrounding file.
func TestEditorLineAcceptsSynthesizedContext(t *testing.T) {
	m := model{mode: viewBranch, focus: focusDiff}
	m.viewLines = []diff.Line{
		{Kind: diff.Hunk, Text: "@@", Path: "a.go"},
		{Kind: diff.Context, Text: "revealed", OldNum: 11, NewNum: 11}, // no Path
		{Kind: diff.Expand, Hidden: 3},
	}
	m.diffCursor = 2
	if got := m.editorLine("a.go"); got != 11 {
		t.Errorf("editorLine = %d, want 11 (revealed context belongs to the file)", got)
	}
}

func TestEditorTargetInHistoryView(t *testing.T) {
	m := model{mode: viewLog, focus: focusDiff}
	m.viewLines = []diff.Line{{Kind: diff.Add, Text: "x", NewNum: 12, Path: "internal/tui/view.go"}}

	if path, _ := m.editorTarget(); path != "" {
		t.Errorf("closed diff pane should have no target, got %q", path)
	}

	m.logDiffOpen = true
	path, line := m.editorTarget()
	if path != "internal/tui/view.go" || line != 12 {
		t.Errorf("editorTarget() = %q:%d, want internal/tui/view.go:12", path, line)
	}
}

// The Stats pages have no file in scope, an empty tree has no file under the
// cursor, and a path that left the working tree must report itself rather than
// silently doing nothing.
func TestOpenEditorGuards(t *testing.T) {
	m := model{mode: viewOverview, editor: "vim"}
	if cmd := m.openEditor(); cmd != nil || m.flash != "" {
		t.Errorf("stats view should be inert, cmd=%v flash=%q", cmd, m.flash)
	}

	m = model{mode: viewBranch, editor: "vim"}
	if cmd := m.openEditor(); cmd != nil || m.flash != "" {
		t.Errorf("empty tree should be inert, cmd=%v flash=%q", cmd, m.flash)
	}

	// A file is in scope, but it isn't in the working tree.
	m = model{mode: viewBranch, editor: "nvim", repo: t.TempDir()}
	ghost := git.FileChange{Path: "ghost.go", Status: "D"}
	m.rows = []treeRow{{file: &ghost, path: ghost.Path}}
	m.cursor = 0
	if cmd := m.openEditor(); cmd != nil {
		t.Errorf("a path off disk should not launch anything, got %v", cmd)
	}
	if m.flash == "" {
		t.Error("a path off disk should explain itself in the footer")
	}
}

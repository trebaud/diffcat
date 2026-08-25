package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
)

// editor.go opens the file under the cursor in the reader's own editor (`e`),
// jumping to the line the diff cursor is on. The editor is whatever the config
// file, the environment, or the --editor flag names, defaulting to vim; diffcat
// suspends itself while it runs (tea.ExecProcess hands over the terminal) and
// refreshes from disk on the way back, so an edit made in vim shows up in the
// diff immediately.

// editorFinishedMsg lands when the suspended editor process exits.
type editorFinishedMsg struct{ err error }

// defaultEditor is what `e` opens when nothing else names an editor. vim is the
// safe assumption for a terminal-first tool: it's present on essentially every
// system diffcat runs on, and its `+N` line syntax is the one this file's
// fallback already speaks.
const defaultEditor = "vim"

// resolveEditor picks the editor command, in precedence order: the --editor
// flag, $DIFFCAT_EDITOR, the saved config, then the conventional $VISUAL /
// $EDITOR, and vim if none of those say anything.
func resolveEditor(flag string, cfg userConfig) string {
	return strings.TrimSpace(firstNonEmpty(
		flag,
		os.Getenv("DIFFCAT_EDITOR"),
		cfg.Editor,
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		defaultEditor,
	))
}

// Editors grouped by how they take a line number on the command line. Anything
// unrecognized is handed just the path — opening at line 1 beats not opening.
var (
	// `editor +N file` — the vim family and most terminal editors.
	editorsPlusLine = map[string]bool{
		"vim": true, "nvim": true, "vi": true, "view": true, "gvim": true,
		"nano": true, "pico": true, "micro": true, "kak": true, "joe": true,
		"emacs": true, "emacsclient": true, "gedit": true,
	}
	// `editor --goto file:N` — the VS Code family.
	editorsGotoFlag = map[string]bool{
		"code": true, "code-insiders": true, "codium": true, "vscodium": true,
		"cursor": true, "windsurf": true,
	}
	// `editor file:N` — editors that take the line glued to the path.
	editorsColonLine = map[string]bool{
		"subl": true, "sublime_text": true, "hx": true, "helix": true, "zed": true,
	}
)

// editorArgv builds the argv for opening path at line. cmd is the configured
// command, which may carry its own flags (e.g. "code -w"); it's split on
// whitespace, so quoted paths inside it are not supported. line <= 0 opens the
// file without jumping.
func editorArgv(cmd, path string, line int) []string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	if line <= 0 {
		return append(parts, path)
	}
	n := strconv.Itoa(line)
	switch name := editorName(parts[0]); {
	case editorsPlusLine[name]:
		return append(parts, "+"+n, path)
	case editorsGotoFlag[name]:
		return append(parts, "--goto", path+":"+n)
	case editorsColonLine[name]:
		return append(parts, path+":"+n)
	default:
		return append(parts, path)
	}
}

// editorName reduces a command to its bare name for the lookup tables:
// "/usr/local/bin/nvim" and "nvim.exe" both resolve to "nvim".
func editorName(cmd string) string {
	base := filepath.Base(cmd)
	return strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
}

// rowLine returns the diff line at display row i, in whichever view mode is
// active. In split mode the new side is what the editor cares about, but a row
// may only have a left (removed) or a full-width (hunk/meta) line — they carry
// no new-side number, yet they do carry the path the boundary check needs.
func (m model) rowLine(i int) *diff.Line {
	if m.splitView {
		if i < 0 || i >= len(m.splitRows) {
			return nil
		}
		r := m.splitRows[i]
		switch {
		case r.Right != nil:
			return r.Right
		case r.Full != nil:
			return r.Full
		default:
			return r.Left
		}
	}
	if i < 0 || i >= len(m.viewLines) {
		return nil
	}
	return &m.viewLines[i]
}

// editorLine is the new-side line number the editor should jump to for path:
// the line under the diff cursor, or the nearest one above it. The cursor may
// rest on a removed line, a hunk header, or an expand affordance — none of which
// exist in the file on disk — so it walks back to the closest line that does.
//
// Returns 0 (open the file, don't jump) when the file list has focus — the diff
// pane only draws its cursor while focused, so there is no line the reader is
// pointing at — or when the walk reaches the start of path's section in a
// combined multi-file patch without finding one.
func (m model) editorLine(path string) int {
	if m.focus != focusDiff {
		return 0
	}
	for i := m.diffCursor; i >= 0; i-- {
		l := m.rowLine(i)
		if l == nil {
			continue
		}
		// The history view's diff pane is one patch spanning several files. Stop at
		// the file boundary rather than walking into the previous file and handing
		// the editor its line numbers. Rows synthesized by BuildView (revealed
		// context, expand affordances) carry no path and belong to the file around
		// them, so they don't count as a boundary.
		if path != "" && l.Path != "" && l.Path != path {
			return 0
		}
		if l.NewNum > 0 {
			return l.NewNum
		}
	}
	return 0
}

// editorTarget resolves which file `e` opens and at what line: the repo-relative
// path plus a line number (0 = don't jump). In the history view the diff pane is
// a whole-commit patch, so the file comes from the line under the cursor (each
// parsed line carries the path from the patch's headers) rather than from a tree
// selection; elsewhere it's the file the tree cursor is on. Returns "" when
// there is nothing to open.
func (m model) editorTarget() (string, int) {
	if m.mode == viewLog {
		if !m.logDiffOpen {
			return "", 0
		}
		if l := m.cursorLine(); l != nil && l.Path != "" {
			return l.Path, m.editorLine(l.Path)
		}
		return "", 0
	}
	if f := m.selectedFile(); f != nil {
		return f.Path, m.editorLine(f.Path)
	}
	return "", 0
}

// openEditor suspends the TUI and runs the configured editor on the file under
// the cursor. A path that is no longer in the working tree says so in the footer
// rather than failing silently; an editor that won't launch (vim absent on a bare
// system, a bad --editor value) reports itself the same way via
// editorFinishedMsg.
func (m *model) openEditor() tea.Cmd {
	if m.mode == viewOverview || m.mode == viewAuthorDetail {
		return nil // the Stats pages have no file in scope
	}
	path, line := m.editorTarget()
	if path == "" {
		return nil
	}
	abs := filepath.Join(m.repo, path)
	// The diff can name a path that isn't in the working tree: a deleted file, or
	// one that only exists in an older commit being browsed.
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		m.flash = path + " is not in the working tree — nothing to open"
		return nil
	}
	argv := editorArgv(m.editor, abs, line)
	if len(argv) == 0 {
		return nil
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = m.repo
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorFinishedMsg{err: err} })
}

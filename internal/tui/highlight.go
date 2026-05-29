package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Syntax highlighting layers token foreground colors over the diff's row tints.
// chromaStyle maps token types → colors; it's swapped per light/dark in
// ApplyTheme to match the GitHub palette already used for the diff backgrounds.
var chromaStyle *chroma.Style = styles.Get("github-dark")

// applyHighlightTheme picks the chroma style matching the terminal background.
func applyHighlightTheme(isDark bool) {
	name := "github"
	if isDark {
		name = "github-dark"
	}
	chromaStyle = styles.Get(name) // never nil — Get falls back internally
}

// span is a run of code sharing one foreground color (nil = terminal default).
// Backgrounds aren't stored here: the diff row tint is applied at render time so
// the same highlighted tokens read on an added, removed, or context line.
type span struct {
	text string
	fg   color.Color
}

// lexerFor picks a syntax lexer from a file path, coalescing adjacent same-type
// tokens. Returns nil when no lexer matches (rendered as plain, uncolored text).
func lexerFor(path string) chroma.Lexer {
	l := lexers.Match(path)
	if l == nil {
		return nil
	}
	return chroma.Coalesce(l)
}

// highlight tokenizes one line into colored spans, memoized per line text so a
// repaint (the nyan tick fires ~7fps) doesn't re-lex the visible window. The
// cache is reset whenever the selected file — and thus the lexer — changes.
func (m model) highlight(text string) []span {
	if m.hlCache != nil {
		if s, ok := m.hlCache[text]; ok {
			return s
		}
	}
	s := tokenizeLine(m.lexer, text)
	if m.hlCache != nil {
		m.hlCache[text] = s
	}
	return s
}

// tokenizeLine lexes a single line. Chroma lexers are line-context-free enough
// that per-line tokenization is accurate for the common cases (a fragment inside
// a multi-line string/comment may mis-color, an acceptable tradeoff for a diff).
func tokenizeLine(lexer chroma.Lexer, text string) []span {
	if lexer == nil || text == "" {
		return []span{{text: text}}
	}
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		return []span{{text: text}}
	}
	var spans []span
	for _, t := range it.Tokens() {
		val := strings.TrimRight(t.Value, "\n")
		if val == "" {
			continue
		}
		var fg color.Color
		if c := chromaStyle.Get(t.Type).Colour; c.IsSet() {
			fg = lipgloss.Color(c.String())
		}
		spans = append(spans, span{text: val, fg: fg})
	}
	return spans
}

const tabWidth = 4

// expandTabs replaces tabs with spaces to the next tab stop so width math and
// alignment stay exact — a literal tab would render wider than its measured
// width and shove the layout. Wide runes are counted as one column (rare in code
// and not worth the per-rune width lookup here).
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// cutToWidth trims a string to at most w display columns (no ellipsis).
func cutToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w {
		r = r[:len(r)-1]
	}
	return string(r)
}

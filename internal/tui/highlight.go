package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Syntax highlighting layers token foreground colors over the diff's row tints.
// Rather than read colors from a chroma Style (whose per-language coverage is
// uneven — numbers share the string color, and most identifiers fall back to a
// flat default), we map Chroma's universal token *categories* to a curated
// GitHub-flavored palette in tokenColor. That gives every language the same
// distinct, legible hues per category, tuned for both themes and readable over
// the green/red diff tints.

// hlDark selects the light or dark syntax palette; kept in sync with the diff
// theme via applyHighlightTheme (called from ApplyTheme).
var hlDark = true

// applyHighlightTheme switches the syntax palette to match the terminal
// background. The per-line span cache is reset by the caller (toggleTheme), so
// the new colors take effect on the next paint.
func applyHighlightTheme(isDark bool) { hlDark = isDark }

// pick returns the dark or light variant of a palette color for the active theme.
func pick(light, dark string) color.Color {
	if hlDark {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

// tokenColor maps a Chroma token type to a foreground color, keyed on its
// category so coverage is uniform across languages. A nil result means "no
// explicit color" — plain identifiers and punctuation follow the terminal/canvas
// foreground, which keeps them legible on any background and in either theme.
//
// The palette mirrors GitHub's: coral keywords, purple functions, orange types,
// blue builtins/constants, light-blue strings (distinct from blue numbers), and
// muted-gray comments. More specific types are matched before their categories.
func tokenColor(t chroma.TokenType) color.Color {
	switch {
	// Comments — but preprocessor directives read as keywords, not prose.
	case t == chroma.CommentPreproc, t == chroma.CommentPreprocFile:
		return pick("#cf222e", "#ff7b72")
	case t.InCategory(chroma.Comment):
		return pick("#6e7781", "#8b949e")

	// Literals — numbers kept distinct from strings; escapes pop within strings.
	case t == chroma.LiteralStringEscape, t == chroma.LiteralStringInterpol, t == chroma.LiteralStringAffix:
		return pick("#0550ae", "#79c0ff")
	case t.InSubCategory(chroma.LiteralString):
		return pick("#0a3069", "#a5d6ff")
	case t.InSubCategory(chroma.LiteralNumber):
		return pick("#0550ae", "#79c0ff")

	// Keywords & language constants. Type keywords (int, func) read as types;
	// true/false/nil and self/this get the constant/keyword tone.
	case t == chroma.KeywordType:
		return pick("#953800", "#ffa657")
	case t == chroma.KeywordConstant:
		return pick("#0550ae", "#79c0ff")
	case t == chroma.NameBuiltinPseudo, t == chroma.OperatorWord:
		return pick("#cf222e", "#ff7b72")
	case t.InCategory(chroma.Keyword):
		return pick("#cf222e", "#ff7b72")

	// Names — functions purple, types/classes orange, builtins/constants blue,
	// markup tags green.
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic, t == chroma.NameDecorator:
		return pick("#8250df", "#d2a8ff")
	case t == chroma.NameClass, t == chroma.NameException, t == chroma.NameNamespace:
		return pick("#953800", "#ffa657")
	case t == chroma.NameBuiltin:
		return pick("#0550ae", "#79c0ff")
	case t == chroma.NameConstant, t == chroma.NameVariableGlobal, t == chroma.NameVariableMagic:
		return pick("#0550ae", "#79c0ff")
	case t == chroma.NameTag:
		return pick("#116329", "#7ee787")
	case t == chroma.NameAttribute, t == chroma.NameLabel:
		return pick("#0550ae", "#79c0ff")

	default:
		return nil // identifiers, operators, punctuation → terminal/canvas fg
	}
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

// highlight tokenizes one line into colored spans with the given lexer, memoized
// so a repaint (the nyan tick fires ~7fps) doesn't re-lex the visible window. The
// cache key folds in the lexer name: a combined multi-file patch highlights each
// file with its own lexer, so the same text under two languages must not collide.
// The cache is reset whenever the diff is (re)loaded.
func (m model) highlight(lexer chroma.Lexer, text string) []span {
	key := text
	if lexer != nil {
		key = lexer.Config().Name + "\x00" + text
	}
	if m.hlCache != nil {
		if s, ok := m.hlCache[key]; ok {
			return s
		}
	}
	s := tokenizeLine(lexer, text)
	if m.hlCache != nil {
		m.hlCache[key] = s
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
		fg := tokenColor(t.Type)
		// Coalesce runs sharing a color (after category mapping, neighbours often
		// collapse to the same hue — e.g. punctuation between two plain names) so
		// the rendered line carries fewer style spans.
		if n := len(spans); n > 0 && sameColor(spans[n-1].fg, fg) {
			spans[n-1].text += val
			continue
		}
		spans = append(spans, span{text: val, fg: fg})
	}
	return spans
}

// sameColor reports whether two span foregrounds are interchangeable, treating
// nil (no explicit color) as its own value so coalescing never merges a colored
// run into a default one.
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a == b
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

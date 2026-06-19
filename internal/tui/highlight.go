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
// flat default), we map Chroma's universal token *categories* to the active
// theme's curated syntax slots in tokenColor. That gives every language the same
// distinct, legible hues per category, tuned per theme and readable over the
// green/red diff tints. The colors come from pal (the active Palette), so they
// switch with the theme; the per-line span cache is dropped on a theme change
// (toggleTheme/setTheme) so the new colors take effect on the next paint.

// tokenColor maps a Chroma token type to a foreground color, keyed on its
// category so coverage is uniform across languages. A nil result means "no
// explicit color" — plain identifiers and punctuation follow the terminal/canvas
// foreground, which keeps them legible on any background and in either theme.
//
// The seven syntax slots mirror GitHub's roles: keywords, functions, types,
// numbers/builtins/constants, strings (distinct from numbers), markup tags, and
// muted comments. More specific token types are matched before their categories.
func tokenColor(t chroma.TokenType) color.Color {
	switch {
	// Comments — but preprocessor directives read as keywords, not prose.
	case t == chroma.CommentPreproc, t == chroma.CommentPreprocFile:
		return pal.SynKeyword
	case t.InCategory(chroma.Comment):
		return pal.SynComment

	// Literals — numbers kept distinct from strings; escapes pop within strings.
	case t == chroma.LiteralStringEscape, t == chroma.LiteralStringInterpol, t == chroma.LiteralStringAffix:
		return pal.SynNumber
	case t.InSubCategory(chroma.LiteralString):
		return pal.SynString
	case t.InSubCategory(chroma.LiteralNumber):
		return pal.SynNumber

	// Keywords & language constants. Type keywords (int, func) read as types;
	// true/false/nil and self/this get the constant/keyword tone.
	case t == chroma.KeywordType:
		return pal.SynType
	case t == chroma.KeywordConstant:
		return pal.SynNumber
	case t == chroma.NameBuiltinPseudo, t == chroma.OperatorWord:
		return pal.SynKeyword
	case t.InCategory(chroma.Keyword):
		return pal.SynKeyword

	// Names — functions, types/classes, builtins/constants, markup tags.
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic, t == chroma.NameDecorator:
		return pal.SynFunction
	case t == chroma.NameClass, t == chroma.NameException, t == chroma.NameNamespace:
		return pal.SynType
	case t == chroma.NameBuiltin:
		return pal.SynNumber
	case t == chroma.NameConstant, t == chroma.NameVariableGlobal, t == chroma.NameVariableMagic:
		return pal.SynNumber
	case t == chroma.NameTag:
		return pal.SynTag
	case t == chroma.NameAttribute, t == chroma.NameLabel:
		return pal.SynNumber

	default:
		return nil // identifiers, operators, punctuation → terminal/canvas fg
	}
}

// span is a run of code sharing one foreground color (nil = terminal default)
// and one italic flag. Backgrounds aren't stored here: the diff row tint is
// applied at render time so the same highlighted tokens read on an added,
// removed, or context line. Comments render italic — a near-universal editor
// convention — on terminals that support it (and degrade to upright otherwise).
type span struct {
	text   string
	fg     color.Color
	italic bool
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
		italic := t.Type.InCategory(chroma.Comment)
		// Coalesce runs sharing a color and italic state (after category mapping,
		// neighbours often collapse — e.g. punctuation between two plain names) so
		// the rendered line carries fewer style spans.
		if n := len(spans); n > 0 && sameColor(spans[n-1].fg, fg) && spans[n-1].italic == italic {
			spans[n-1].text += val
			continue
		}
		spans = append(spans, span{text: val, fg: fg, italic: italic})
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

package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"
)

func TestExpandTabs(t *testing.T) {
	cases := map[string]string{
		"\tx":     "    x",  // one tab → next stop at col 4
		"a\tb":    "a   b",  // partial tab fills to col 4
		"ab\tc":   "ab  c",  //
		"abcd\te": "abcd    e",
		"no tabs": "no tabs",
	}
	for in, want := range cases {
		if got := expandTabs(in); got != want {
			t.Errorf("expandTabs(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderCodeWidth guards that the highlighted body always occupies exactly
// the requested width — the no-wrap invariant the diff pane depends on — across
// truncation, padding, and tab-expanded lines, with a real lexer driving colors.
func TestRenderCodeWidth(t *testing.T) {
	ApplyTheme(true)
	m := model{lexer: lexerFor("x.go"), hlCache: map[string][]span{}}
	lines := []string{
		"func newer(n int) string {",
		"\treturn fmt.Sprintf(\"value=%d\", n) // trailing comment that is quite long",
		"",
		"x",
	}
	for _, ln := range lines {
		for _, w := range []int{1, 2, 5, 12, 40, 200} {
			// Exercise both the plain path and the word-emphasis path (a range that
			// straddles a tab) to guard the width math through the mask split.
			out := m.renderCode(ln, m.lexer, w, diffAddBg, nil, nil)
			if got := lipgloss.Width(out); got != w {
				t.Errorf("renderCode(%q, %d) width = %d, want %d", ln, w, got, w)
			}
			emph := m.renderCode(ln, m.lexer, w, diffAddBg, diffAddEmphBg, [][2]int{{2, 9}})
			if got := lipgloss.Width(emph); got != w {
				t.Errorf("renderCode(%q, %d) emph width = %d, want %d", ln, w, got, w)
			}
		}
	}
}

// TestHighlightColorsCode confirms a known Go keyword gets a non-default color
// and that splitting preserves the line's text content.
func TestHighlightColorsCode(t *testing.T) {
	ApplyTheme(true)
	spans := tokenizeLine(lexerFor("x.go"), "func main() {}")
	var rebuilt strings.Builder
	colored := false
	for _, s := range spans {
		rebuilt.WriteString(s.text)
		if s.fg != nil {
			colored = true
		}
	}
	if rebuilt.String() != "func main() {}" {
		t.Errorf("spans lost content: %q", rebuilt.String())
	}
	if !colored {
		t.Error("expected at least one colored span for Go source")
	}
}

// TestTokenColorPalette locks in the curated palette's guarantees: distinct hues
// per category (the old chroma-style lookup made numbers and strings identical
// and most identifiers flat), subcategory fallback so every number/string subtype
// shares its category color, and theme sensitivity.
func TestTokenColorPalette(t *testing.T) {
	applyHighlightTheme(true)

	str := tokenColor(chroma.LiteralStringDouble)
	num := tokenColor(chroma.LiteralNumberInteger)
	kw := tokenColor(chroma.Keyword)
	fn := tokenColor(chroma.NameFunction)
	typ := tokenColor(chroma.NameClass)
	com := tokenColor(chroma.Comment)

	for name, c := range map[string]interface{}{"string": str, "number": num, "keyword": kw, "function": fn, "type": typ, "comment": com} {
		if c == nil {
			t.Errorf("%s token should be colored, got nil", name)
		}
	}

	// The headline fix: numbers and strings are no longer the same color, and the
	// major categories are mutually distinct.
	distinct := []struct {
		name string
		a, b interface{}
	}{
		{"number vs string", num, str},
		{"keyword vs function", kw, fn},
		{"function vs type", fn, typ},
		{"keyword vs string", kw, str},
	}
	for _, d := range distinct {
		if d.a == d.b {
			t.Errorf("%s should differ, both = %v", d.name, d.a)
		}
	}

	// Subcategory fallback: every string/number subtype resolves to its category
	// color, so coverage is uniform regardless of which subtype a lexer emits.
	if tokenColor(chroma.LiteralStringBacktick) != str {
		t.Error("string subtypes should share the string color")
	}
	if tokenColor(chroma.LiteralNumberHex) != num {
		t.Error("number subtypes should share the number color")
	}

	// Theme sensitivity: the light palette differs from the dark one.
	applyHighlightTheme(false)
	if tokenColor(chroma.Keyword) == kw {
		t.Error("keyword color should differ between light and dark themes")
	}
	applyHighlightTheme(true) // restore for other tests
}

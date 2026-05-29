package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
			out := m.renderCode(ln, w, diffAddBg)
			if got := lipgloss.Width(out); got != w {
				t.Errorf("renderCode(%q, %d) width = %d, want %d", ln, w, got, w)
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

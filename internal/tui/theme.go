package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette — semantic colors only. We default to ANSI 16 indices so the user's
// terminal theme controls actual appearance; ApplyTheme overrides the layered
// background/border tones per light/dark detection.
var (
	colAccent  color.Color = lipgloss.Color("5") // magenta
	colAdded   color.Color = lipgloss.Color("2") // green
	colRemoved color.Color = lipgloss.Color("1") // red
	colMeta    color.Color = lipgloss.Color("6") // cyan (hunk headers)
	colWarn    color.Color = lipgloss.Color("3") // yellow

	// These ANSI defaults only stand in before ApplyTheme runs; ApplyTheme swaps
	// in tuned truecolor so the chrome stays on the same designed surface as the
	// diff tints rather than drifting with the terminal's own 16-color palette.
	colMuted  color.Color = lipgloss.ANSIColor(8)
	colRowBg  color.Color = lipgloss.Color("239")
	colBorder color.Color = lipgloss.ANSIColor(8)

	// Interactive state (row selection + pane focus). The same info-blue as hunk
	// headers — kept distinct from the magenta brand/nyan accent so "selected /
	// focused" reads as its own signal, not the app's identity color.
	colSelect color.Color = lipgloss.Color("4")

	// Whole-screen canvas. These drive tea.View's terminal background/foreground
	// so the entire UI — not just the diff's add/del tints — flips with the theme.
	// nil means "leave the terminal's own colors": in dark mode we respect the
	// user's terminal (the long-standing default), but the light theme paints an
	// explicit near-white canvas with dark text, otherwise the dark github syntax
	// colors would have no light background to read against.
	colCanvas color.Color
	colText   color.Color

	// colOverlayBg is the solid background of floating windows (help, commit
	// details) — a slightly elevated panel tone so the window reads as sitting
	// above the dimmed scrim behind it.
	colOverlayBg color.Color
)

var (
	titleStyle       lipgloss.Style
	mutedStyle       lipgloss.Style
	headingStyle     lipgloss.Style
	selectedStyle    lipgloss.Style
	selectedRowStyle lipgloss.Style
	borderStyle      lipgloss.Style

	dirStyle       lipgloss.Style // folder rows in the file tree
	treeGuideStyle lipgloss.Style // the faint │ rails connecting tree levels

	branchStyle    lipgloss.Style // the current branch name in the header
	baseBadgeStyle lipgloss.Style // the base branch, as a colored pill in the header
	tagStyle       lipgloss.Style // git tags in the commit details / history list
	headStyle      lipgloss.Style // local branch refs, e.g. [main]
	remoteStyle    lipgloss.Style // remote-tracking refs, e.g. {origin/main}

	catStyle lipgloss.Style // the nyan cat — brand magenta, kept off the selection blue

	addedStyle   lipgloss.Style
	removedStyle lipgloss.Style
	metaStyle    lipgloss.Style
	contextStyle lipgloss.Style

	// GitHub-style diff line styles. Each kind has a gutter style (line numbers +
	// marker, tinted in the accent color); the code body is rendered by renderCode
	// with the row's background tint (diffAddBg/diffDelBg) so syntax-highlighted
	// tokens sit on one continuous band.
	addNumStyle     lipgloss.Style
	delNumStyle     lipgloss.Style
	ctxNumStyle     lipgloss.Style
	hunkLineStyle   lipgloss.Style
	metaLineStyle   lipgloss.Style
	expandLineStyle lipgloss.Style // "expand hidden context" affordance rows
	fillerStyle     lipgloss.Style // empty paired side in split view

	diffAddBg color.Color // row tint behind added lines
	diffDelBg color.Color // row tint behind removed lines

	// Stronger tints for the exact words that changed within a line — GitHub's
	// two-tone diff. Painted by renderCode over the soft body band so the changed
	// span pops out of the surrounding unchanged code.
	diffAddEmphBg color.Color
	diffDelEmphBg color.Color

	// pulseRamp is a soft dim→bright ramp of the info-blue tone; a file a sync
	// changed but the reader hasn't opened breathes its status glyph through it.
	pulseRamp []color.Color

	// Diff-search highlight: a soft amber band on every match, and a brighter gold
	// on the one under the cursor (the n/N target) — the standard "find" colors,
	// painted by renderCode over the row tint.
	colSearchBg    color.Color
	colSearchCurBg color.Color
)

// pal is the active resolved palette (the light or dark half of the active
// theme). highlight.go reads its syntax slots so token colors track the theme.
var pal Palette

// activeThemeIdx is the index into themes of the active theme — the picker reads
// and advances it; setTheme keeps it in sync with what ApplyTheme last applied.
var activeThemeIdx int

func init() { ApplyTheme(themeGitHub, true) }

// ApplyTheme rebuilds the style table from theme t for the current terminal
// background, selecting t's light or dark palette and assigning every semantic
// color from it. The style-construction below is theme-agnostic: it reads only
// the resolved color vars, so adding a theme is purely a matter of supplying a
// Palette.
func ApplyTheme(t Theme, isDark bool) {
	activeThemeIdx = themeIndex(t)
	p := t.Dark
	if !isDark {
		p = t.Light
	}
	pal = p

	// Brand + semantic hues. GitHub keeps these on the ANSI 16 indices so the
	// user's terminal palette drives them (the long-standing default); other
	// themes supply tuned truecolor.
	colAccent = p.Accent
	colAdded = p.Added
	colRemoved = p.Removed
	colMeta = p.Meta
	colWarn = p.Warn

	// Tuned neutrals. colMuted is the same gray the syntax highlighter uses for
	// comments (~4.6:1 on the dark canvas — well above the ANSI-8 it replaced,
	// which rendered at whatever low contrast the terminal theme chose).
	colMuted = p.Muted
	colRowBg = p.RowBg
	colBorder = p.Border
	colSelect = p.Select

	// Light mode typically paints an explicit canvas (a page bg + ink); dark
	// mode keeps nil so the user's terminal background shows through. A palette
	// signals "respect the terminal" with a nil Canvas.
	colCanvas, colText = p.Canvas, p.Text
	colOverlayBg = p.OverlayBg

	addBg, addGut, addFg := p.AddBg, p.AddGut, p.AddFg
	delBg, delGut, delFg := p.DelBg, p.DelGut, p.DelFg
	hunkBg, hunkFg := p.HunkBg, p.HunkFg
	fillBg := p.FillBg
	expandBg := p.ExpandBg

	titleStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	catStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(colMuted)
	headingStyle = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	// Selection + pane focus read in the info-blue (colSelect), not the brand
	// magenta, so interactive state is its own signal.
	selectedStyle = lipgloss.NewStyle().Foreground(colSelect).Bold(true)
	// Full-width selection bar. A layered background reads on every color tier
	// (degrades to a visible block even in 16-color mode) and never relies on
	// color alone — the ▸ caret marks the row too.
	selectedRowStyle = lipgloss.NewStyle().Background(colRowBg).Foreground(colSelect).Bold(true)
	borderStyle = lipgloss.NewStyle().Foreground(colBorder)

	// Folders read in the cyan "meta" tone (distinct from the magenta selection
	// accent); the tree rails are as faint as the pane divider.
	dirStyle = lipgloss.NewStyle().Foreground(colMeta).Bold(true)
	treeGuideStyle = lipgloss.NewStyle().Foreground(colBorder)

	addedStyle = lipgloss.NewStyle().Foreground(colAdded)
	removedStyle = lipgloss.NewStyle().Foreground(colRemoved)
	metaStyle = lipgloss.NewStyle().Foreground(colMeta).Bold(true)
	contextStyle = lipgloss.NewStyle().Foreground(colMuted)

	// Diff tints are two-tier, GitHub-style: a deep, saturated *body* band the code
	// text sits on, and a brighter, more vivid *gutter* band behind the line
	// numbers and the +/- marker. The lightness gap between the two is what gives
	// the diff its fluorescent depth on a dark canvas. GitHub's dark values are
	// tuned in OKLCH at one constant body lightness (L≈0.30, chroma pushed to the
	// gamut edge) so add/del/hunk read as a single balanced set rather than three
	// mismatched browns; light values are GitHub's own. All map to the nearest
	// 256/16 color where truecolor isn't available.
	diffAddBg, diffDelBg = addBg, delBg
	// The word-level emphasis reuses the brighter gutter band: GitHub's intra-line
	// highlight (#abf2bc / #ffc1bc in light) is exactly that tone, and on the dark
	// theme the gutter green/red already reads as a stronger step above the body.
	diffAddEmphBg, diffDelEmphBg = addGut, delGut

	// A gentle breathing ramp toward the selection accent — dim to bright, so an
	// unopened changed file's glyph eases in and out rather than blinking. GitHub
	// supplies six hand-tuned OKLCH stops; other themes derive a ramp from their
	// own neutral→accent so the breath stays on-palette.
	if len(p.Pulse) >= 2 {
		pulseRamp = p.Pulse
	} else {
		pulseRamp = derivePulse(p.RowBg, p.Select)
	}

	// Search highlight: a find-on-page amber band, the current match a brighter
	// gold so n/N's target stands out. Tuned per theme so token foregrounds stay
	// legible over the band.
	colSearchBg = p.SearchBg
	colSearchCurBg = p.SearchCurBg

	// Header: the current branch reads bright (inherits the terminal fg, just
	// bold), while the base branch is a blue "info" pill — the same GitHub blue
	// as hunk headers — so it's obvious at a glance what the diff is measured
	// against. The pill degrades to a visible block in 16-color terminals.
	branchStyle = lipgloss.NewStyle().Bold(true)
	baseBadgeStyle = lipgloss.NewStyle().Background(hunkBg).Foreground(hunkFg).Bold(true)

	// Git tags read in a subtle gold — git's own decoration color for tags, kept
	// muted (not bold) so they sit quietly beside the brighter subject/SHA.
	tagStyle = lipgloss.NewStyle().Foreground(p.Tag)

	// Branch decorations track git's own ref colors so the badges read the way
	// `git log --decorate` and tig do: local branches green, remote-tracking refs
	// red. The checked-out branch ([main]) is bold to set it apart from the rest.
	headStyle = lipgloss.NewStyle().Foreground(colAdded).Bold(true)
	remoteStyle = lipgloss.NewStyle().Foreground(colRemoved)

	// The number/marker gutter rides the brighter gutter band; the code body uses
	// diffAddBg/diffDelBg (the deeper body band) in renderCode.
	addNumStyle = lipgloss.NewStyle().Background(addGut).Foreground(addFg)
	delNumStyle = lipgloss.NewStyle().Background(delGut).Foreground(delFg)
	ctxNumStyle = lipgloss.NewStyle().Foreground(colMuted)
	hunkLineStyle = lipgloss.NewStyle().Background(hunkBg).Foreground(hunkFg).Bold(true)
	metaLineStyle = lipgloss.NewStyle().Foreground(colMuted)
	expandLineStyle = lipgloss.NewStyle().Background(expandBg).Foreground(hunkFg)
	fillerStyle = lipgloss.NewStyle().Background(fillBg)

	// Syntax-highlight token colors are read live from pal, so there's nothing
	// further to apply here — but callers still drop the per-line span cache
	// (setTheme) so the new colors take on the next paint.
}

// derivePulse builds a six-stop dim→bright breathing ramp from a dim neutral
// toward the accent, for themes that don't supply their own hand-tuned stops.
// A nil accent (the monochrome theme) yields a nil ramp — pulseShade then falls
// back to the meta tone, so the glyph simply doesn't breathe in color.
func derivePulse(dim, accent color.Color) []color.Color {
	if accent == nil {
		return nil
	}
	if dim == nil {
		dim = accent
	}
	const stops = 6
	ramp := make([]color.Color, stops)
	for i := range ramp {
		// Ease from the dim neutral (i=0) up to the full accent (i=stops-1).
		t := float64(i) / float64(stops-1)
		ramp[i] = blendColor(dim, accent, t)
	}
	return ramp
}

// statusGlyph maps a git status letter to a single-character badge.
func statusGlyph(status string) string {
	switch status {
	case "A":
		return "+"
	case "D":
		return "-"
	case "R", "C":
		return "»"
	case "?":
		return "?"
	default:
		return "~"
	}
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "A", "?":
		return addedStyle
	case "D":
		return removedStyle
	case "R", "C":
		return metaStyle
	default:
		return mutedStyle
	}
}

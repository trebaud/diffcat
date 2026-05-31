package tui

import (
	"image/color"
	"os"

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

	// pulseRamp is a soft dim→bright ramp of the info-blue tone; a file a sync
	// changed but the reader hasn't opened breathes its status glyph through it.
	pulseRamp []color.Color
)

func init() { ApplyTheme(true) }

// ApplyTheme rebuilds the style table for the current terminal background.
func ApplyTheme(isDark bool) {
	ld := lipgloss.LightDark(isDark)

	// Tuned truecolor neutrals. colMuted is the same gray the syntax highlighter
	// uses for comments (~4.6:1 on the dark canvas — well above the ANSI-8 it
	// replaced, which rendered at whatever low contrast the terminal theme chose).
	colMuted = ld(lipgloss.Color("#6e7781"), lipgloss.Color("#8b949e"))
	colRowBg = ld(lipgloss.Color("#eaeef2"), lipgloss.Color("#21262d"))
	colBorder = ld(lipgloss.Color("#d0d7de"), lipgloss.Color("#30363d"))
	colSelect = ld(lipgloss.Color("#0969da"), lipgloss.Color("#58a6ff"))

	// Light mode paints an explicit canvas (GitHub's page bg + ink); dark mode
	// keeps nil so the user's terminal background shows through, as before.
	if isDark {
		colCanvas, colText = nil, nil
	} else {
		colCanvas, colText = lipgloss.Color("#ffffff"), lipgloss.Color("#1f2328")
	}
	colOverlayBg = ld(lipgloss.Color("#ffffff"), lipgloss.Color("#161b22"))

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
	// the diff its fluorescent depth on a dark canvas. The dark values are tuned in
	// OKLCH at one constant body lightness (L≈0.30, chroma pushed to the gamut
	// edge) so add/del/hunk read as a single balanced set rather than three
	// mismatched browns; light values are GitHub's own. Code text keeps ≥8:1 on the
	// dark body, markers ≥4.3:1 on their gutter. All map to the nearest 256/16
	// color where truecolor isn't available.
	addBg := ld(lipgloss.Color("#e6ffec"), lipgloss.Color("#003914"))
	addGut := ld(lipgloss.Color("#abf2bc"), lipgloss.Color("#00672c"))
	addFg := ld(lipgloss.Color("#1a7f37"), lipgloss.Color("#7fe998"))
	delBg := ld(lipgloss.Color("#ffebe9"), lipgloss.Color("#5d0003"))
	delGut := ld(lipgloss.Color("#ffc1bc"), lipgloss.Color("#a4000d"))
	delFg := ld(lipgloss.Color("#cf222e"), lipgloss.Color("#ffa598"))
	hunkBg := ld(lipgloss.Color("#ddf4ff"), lipgloss.Color("#002c60"))
	hunkFg := ld(lipgloss.Color("#0969da"), lipgloss.Color("#88d1ff"))
	fillBg := ld(lipgloss.Color("#f6f8fa"), lipgloss.Color("#0d1117"))
	// Expand affordance: a faint cool band, just distinct enough from the canvas to
	// read as a row (the old fill tint was the canvas itself, so it vanished).
	expandBg := ld(lipgloss.Color("#eef4fb"), lipgloss.Color("#12202f"))

	diffAddBg, diffDelBg = addBg, delBg

	// A gentle breathing ramp toward the same info-blue as hunk headers — dim to
	// bright, so the glyph eases in and out rather than blinking. Six stops on a
	// perceptually-even OKLCH path (hue 250, lightness + chroma rising together),
	// so the breath reads as a smooth fade rather than the visible steps a short
	// ramp gives. Distinct from the magenta brand and the add/del green/red.
	pulseRamp = []color.Color{
		ld(lipgloss.Color("#b9c9db"), lipgloss.Color("#233447")),
		ld(lipgloss.Color("#98b7d8"), lipgloss.Color("#304b67")),
		ld(lipgloss.Color("#77a5d4"), lipgloss.Color("#3d6389")),
		ld(lipgloss.Color("#5392d0"), lipgloss.Color("#4a7cad")),
		ld(lipgloss.Color("#297ecb"), lipgloss.Color("#5895d3")),
		ld(lipgloss.Color("#006ac5"), lipgloss.Color("#67b0f9")),
	}

	// Header: the current branch reads bright (inherits the terminal fg, just
	// bold), while the base branch is a blue "info" pill — the same GitHub blue
	// as hunk headers — so it's obvious at a glance what the diff is measured
	// against. The pill degrades to a visible block in 16-color terminals.
	branchStyle = lipgloss.NewStyle().Bold(true)
	baseBadgeStyle = lipgloss.NewStyle().Background(hunkBg).Foreground(hunkFg).Bold(true)

	// The number/marker gutter rides the brighter gutter band; the code body uses
	// diffAddBg/diffDelBg (the deeper body band) in renderCode.
	addNumStyle = lipgloss.NewStyle().Background(addGut).Foreground(addFg)
	delNumStyle = lipgloss.NewStyle().Background(delGut).Foreground(delFg)
	ctxNumStyle = lipgloss.NewStyle().Foreground(colMuted)
	hunkLineStyle = lipgloss.NewStyle().Background(hunkBg).Foreground(hunkFg).Bold(true)
	metaLineStyle = lipgloss.NewStyle().Foreground(colMuted)
	expandLineStyle = lipgloss.NewStyle().Background(expandBg).Foreground(hunkFg)
	fillerStyle = lipgloss.NewStyle().Background(fillBg)

	// Syntax-highlight palette tracks the same light/dark choice.
	applyHighlightTheme(isDark)
}

// DetectAndApplyTheme inspects the terminal background once and applies the
// matching theme, returning whether the dark theme was chosen. Falls back to
// dark on any detection error.
func DetectAndApplyTheme() bool {
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	ApplyTheme(dark)
	return dark
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

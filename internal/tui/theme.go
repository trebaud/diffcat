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

	colMuted  color.Color = lipgloss.ANSIColor(8)
	colRowBg  color.Color = lipgloss.Color("239")
	colBorder color.Color = lipgloss.ANSIColor(8)
)

var (
	titleStyle       lipgloss.Style
	mutedStyle       lipgloss.Style
	headingStyle     lipgloss.Style
	selectedStyle    lipgloss.Style
	selectedRowStyle lipgloss.Style
	borderStyle      lipgloss.Style

	addedStyle   lipgloss.Style
	removedStyle lipgloss.Style
	metaStyle    lipgloss.Style
	contextStyle lipgloss.Style

	// GitHub-style diff line styles. Each kind has a gutter style (line numbers
	// + marker, tinted in the accent color) and a body style (code on a tinted
	// background). Same background across gutter+body so the row tint reads as
	// one continuous band.
	addNumStyle   lipgloss.Style
	addBodyStyle  lipgloss.Style
	delNumStyle   lipgloss.Style
	delBodyStyle  lipgloss.Style
	ctxNumStyle   lipgloss.Style
	ctxBodyStyle  lipgloss.Style
	hunkLineStyle lipgloss.Style
	metaLineStyle lipgloss.Style
	fillerStyle   lipgloss.Style // empty paired side in split view
)

func init() { ApplyTheme(true) }

// ApplyTheme rebuilds the style table for the current terminal background.
func ApplyTheme(isDark bool) {
	ld := lipgloss.LightDark(isDark)

	colRowBg = ld(lipgloss.Color("254"), lipgloss.Color("239"))
	colBorder = ld(lipgloss.Color("250"), lipgloss.Color("238"))

	titleStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(colMuted)
	headingStyle = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	// Full-width selection bar. A layered background reads on every color tier
	// (degrades to a visible block even in 16-color mode) and never relies on
	// color alone — the ▸ caret marks the row too.
	selectedRowStyle = lipgloss.NewStyle().Background(colRowBg).Foreground(colAccent).Bold(true)
	borderStyle = lipgloss.NewStyle().Foreground(colBorder)

	addedStyle = lipgloss.NewStyle().Foreground(colAdded)
	removedStyle = lipgloss.NewStyle().Foreground(colRemoved)
	metaStyle = lipgloss.NewStyle().Foreground(colMeta).Bold(true)
	contextStyle = lipgloss.NewStyle().Foreground(colMuted)

	// GitHub-like tints. Hex values are exact on truecolor terminals and map to
	// the nearest 256/16 color elsewhere; the +/- marker and line numbers keep
	// the change legible even where backgrounds degrade.
	addBg := ld(lipgloss.Color("#e6ffec"), lipgloss.Color("#12261c"))
	addFg := ld(lipgloss.Color("#1a7f37"), lipgloss.Color("#3fb950"))
	delBg := ld(lipgloss.Color("#ffebe9"), lipgloss.Color("#27191c"))
	delFg := ld(lipgloss.Color("#cf222e"), lipgloss.Color("#f85149"))
	hunkBg := ld(lipgloss.Color("#ddf4ff"), lipgloss.Color("#13243b"))
	hunkFg := ld(lipgloss.Color("#0969da"), lipgloss.Color("#58a6ff"))
	fillBg := ld(lipgloss.Color("#f6f8fa"), lipgloss.Color("#0d1117"))

	addNumStyle = lipgloss.NewStyle().Background(addBg).Foreground(addFg)
	addBodyStyle = lipgloss.NewStyle().Background(addBg)
	delNumStyle = lipgloss.NewStyle().Background(delBg).Foreground(delFg)
	delBodyStyle = lipgloss.NewStyle().Background(delBg)
	ctxNumStyle = lipgloss.NewStyle().Foreground(colMuted)
	ctxBodyStyle = lipgloss.NewStyle()
	hunkLineStyle = lipgloss.NewStyle().Background(hunkBg).Foreground(hunkFg).Bold(true)
	metaLineStyle = lipgloss.NewStyle().Foreground(colMuted)
	fillerStyle = lipgloss.NewStyle().Background(fillBg)
}

// DetectAndApplyTheme inspects the terminal background once and applies the
// matching theme. Falls back to dark on any detection error.
func DetectAndApplyTheme() {
	ApplyTheme(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
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

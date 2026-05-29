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
	titleStyle    lipgloss.Style
	mutedStyle    lipgloss.Style
	headingStyle  lipgloss.Style
	selectedStyle lipgloss.Style
	borderStyle   lipgloss.Style

	addedStyle   lipgloss.Style
	removedStyle lipgloss.Style
	metaStyle    lipgloss.Style
	contextStyle lipgloss.Style
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
	borderStyle = lipgloss.NewStyle().Foreground(colBorder)

	addedStyle = lipgloss.NewStyle().Foreground(colAdded)
	removedStyle = lipgloss.NewStyle().Foreground(colRemoved)
	metaStyle = lipgloss.NewStyle().Foreground(colMeta).Bold(true)
	contextStyle = lipgloss.NewStyle().Foreground(colMuted)
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

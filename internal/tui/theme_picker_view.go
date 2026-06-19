package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// theme_picker_view.go renders the theme picker (the `t` overlay): the list of
// registered themes with a per-theme swatch strip, plus live toggles for the
// light/dark variant, the icon tier, and reduce-motion. Selection is shown with
// a caret + bold name (not a full bar) so each theme's swatches keep their own
// colors while highlighted.

// themePickerBox builds the picker as a floating window.
func (m model) themePickerBox() string {
	w := m.width - 6
	if w > 48 {
		w = 48
	}
	if w < 28 {
		w = 28
	}

	variant := "dark"
	if !m.dark {
		variant = "light"
	}

	lines := []string{
		padLine(titleStyle.Render("theme")+mutedStyle.Render("  "+variant), w),
		padLine("", w),
	}
	for i, t := range themes {
		lines = append(lines, padLine(themePickerRow(t, i == m.themeSel, m.dark), w))
	}
	lines = append(lines,
		padLine("", w),
		padLine(mutedStyle.Render("icons: ")+selectedStyle.Render(m.iconSet.String())+
			mutedStyle.Render("   motion: ")+selectedStyle.Render(onOff(!m.reduceMotion)), w),
		padLine("", w),
		padLine(mutedStyle.Render("j/k preview · T light/dark · i icons · m motion"), w),
		padLine(mutedStyle.Render("↵ keep · esc cancel"), w),
	)
	return floatingBox(strings.Join(lines, "\n"))
}

// themePickerRow renders one theme: a selection caret, the name, then a swatch
// strip of the theme's brand/add/remove/meta/select hues for the active variant.
func themePickerRow(t Theme, selected bool, dark bool) string {
	marker := "  "
	nameStyle := mutedStyle
	if selected {
		marker = selectedStyle.Render("▌ ")
		nameStyle = selectedStyle
	}
	p := t.Dark
	if !dark {
		p = t.Light
	}
	name := nameStyle.Render(fmt.Sprintf("%-12s", t.Name))
	return marker + name + " " + swatchStrip(p)
}

// swatchStrip renders five color blocks for a palette; a nil slot (Monochrome)
// reads as faint dots so the row still aligns.
func swatchStrip(p Palette) string {
	blk := func(c color.Color) string {
		if c == nil {
			return mutedStyle.Render("··")
		}
		return lipgloss.NewStyle().Foreground(c).Render("██")
	}
	cols := []color.Color{p.Accent, p.Added, p.Removed, p.Meta, p.Select}
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = blk(c)
	}
	return strings.Join(parts, " ")
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

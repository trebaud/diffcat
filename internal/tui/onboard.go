package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// onboard.go holds the playful, non-essential flourishes: the one-time first-run
// hint toast. It's gated behind reduce-motion at the source (newModel skips arming
// it), so this file only handles how it looks, not whether it runs.

// toastFrames is how many frames the first-run hint lingers before fading out.
const toastFrames = 30

// rainbowPalette is the nyan trail's perceptually-even rainbow (constant OKLCH
// L=0.72 C=0.155, hue rotating), so every rainbow in the UI matches.
var rainbowPalette = []color.Color{
	lipgloss.Color("#f67972"), lipgloss.Color("#e19005"),
	lipgloss.Color("#5ebd64"), lipgloss.Color("#00c1c2"),
	lipgloss.Color("#69a3ff"), lipgloss.Color("#d480da"),
}

// rainbowBar renders n cells of the shimmering rainbow trail in 2-char blocks
// whose colors rotate by frame, so a static call looks banded and a ticking one
// shimmers.
func rainbowBar(n, frame int) string {
	if n <= 0 {
		return ""
	}
	if frame < 0 {
		frame = -frame
	}
	var b strings.Builder
	for blk := 0; blk*2 < n; blk++ {
		start := blk * 2
		end := start + 2
		if end > n {
			end = n
		}
		s := lipgloss.NewStyle().Foreground(rainbowPalette[(blk+frame)%len(rainbowPalette)])
		b.WriteString(s.Render(strings.Repeat("━", end-start)))
	}
	return b.String()
}

// toastText is the one-time first-run hint that briefly replaces the footer,
// pointing at help and global search. It dims as it ages so the fade-out
// reads even on a single line.
func (m model) toastText() string {
	hint := "✨ new here? press ?  for help  ·  ctrl+k  to search commits, files & code"
	age := m.animFrame - m.toastStart
	// Brighten early, settle to muted as it nears its end, so clearing reads as a
	// fade rather than a snap.
	if age > toastFrames*2/3 {
		return mutedStyle.Render(hint)
	}
	return selectedStyle.Render(hint)
}

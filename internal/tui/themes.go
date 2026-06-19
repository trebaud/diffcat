package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// themes.go is the theme registry. Each Theme carries a light and a dark Palette;
// ApplyTheme picks the half matching the terminal background and assigns it to the
// global color/style table. GitHub (the default) is spelled out verbatim so its
// output is byte-identical to the original hard-coded theme; the rest are derived
// from a compact set of source hues (paletteFrom), which keeps the diff tints and
// search bands on-palette without hand-tuning thirty colors apiece.

// Palette is the full set of resolved colors for one theme variant (light or
// dark). A nil color means "leave it to the terminal" (used by Monochrome and by
// GitHub's dark canvas/text).
type Palette struct {
	// Brand + semantic hues.
	Accent  color.Color // brand magenta / nyan
	Added   color.Color // additions, local branches, A/? status
	Removed color.Color // deletions, remote refs, D status
	Meta    color.Color // folders, renames, hunk-header tone
	Warn    color.Color // merge-commit glyph

	// Neutrals + interactive state.
	Muted     color.Color
	RowBg     color.Color // selection bar background
	Border    color.Color
	Select    color.Color // selection/focus accent
	Canvas    color.Color // whole-screen background (nil = terminal default)
	Text      color.Color // whole-screen foreground (nil = terminal default)
	OverlayBg color.Color // floating-window panel background

	// Diff tints (two-tier: a deep body band + a brighter gutter band).
	AddBg, AddGut, AddFg color.Color
	DelBg, DelGut, DelFg color.Color
	HunkBg, HunkFg       color.Color
	FillBg               color.Color // empty paired side in split view
	ExpandBg             color.Color // "expand hidden context" rows

	// Search highlight + git-tag gold.
	SearchBg, SearchCurBg color.Color
	Tag                   color.Color

	// Pulse breathing ramp for unopened changed files. Nil → derived from
	// RowBg→Select at apply time.
	Pulse []color.Color

	// Syntax token colors (read live by highlight.go's tokenColor).
	SynComment, SynString, SynNumber, SynKeyword, SynType, SynFunction, SynTag color.Color
}

// Theme is a named light/dark palette pair.
type Theme struct {
	Name  string
	Light Palette
	Dark  Palette
}

// themes is the ordered registry the picker cycles through; index 0 is the
// default.
var themes = []Theme{
	themeGitHub,
	themeDracula,
	themeNord,
	themeCatppuccin,
	themeGruvbox,
	themeTokyoNight,
	themeSolarized,
	themeMonochrome,
}

// themeIndex returns t's position in the registry, or 0 (GitHub) if it isn't a
// registered theme.
func themeIndex(t Theme) int {
	for i := range themes {
		if themes[i].Name == t.Name {
			return i
		}
	}
	return 0
}

// themeIndexByName resolves a theme name (case-insensitive, spaces/dashes
// ignored) to its registry index, or -1 if unknown.
func themeIndexByName(name string) int {
	norm := func(s string) string {
		return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s))
	}
	want := norm(name)
	for i := range themes {
		if norm(themes[i].Name) == want {
			return i
		}
	}
	return -1
}

// c is a terse hex-color constructor.
func c(hex string) color.Color { return lipgloss.Color(hex) }

// --- GitHub (default) — verbatim values from the original hard-coded theme ---

var themeGitHub = Theme{
	Name: "GitHub",
	Light: Palette{
		Accent: c("5"), Added: c("2"), Removed: c("1"), Meta: c("6"), Warn: c("3"),
		Muted: c("#6e7781"), RowBg: c("#eaeef2"), Border: c("#d0d7de"), Select: c("#0969da"),
		Canvas: c("#ffffff"), Text: c("#1f2328"), OverlayBg: c("#ffffff"),
		AddBg: c("#e6ffec"), AddGut: c("#abf2bc"), AddFg: c("#1a7f37"),
		DelBg: c("#ffebe9"), DelGut: c("#ffc1bc"), DelFg: c("#cf222e"),
		HunkBg: c("#ddf4ff"), HunkFg: c("#0969da"),
		FillBg: c("#f6f8fa"), ExpandBg: c("#eef4fb"),
		SearchBg: c("#fff8c5"), SearchCurBg: c("#f6c343"), Tag: c("#9a6700"),
		Pulse:      []color.Color{c("#b9c9db"), c("#98b7d8"), c("#77a5d4"), c("#5392d0"), c("#297ecb"), c("#006ac5")},
		SynComment: c("#6e7781"), SynString: c("#0a3069"), SynNumber: c("#0550ae"),
		SynKeyword: c("#cf222e"), SynType: c("#953800"), SynFunction: c("#8250df"), SynTag: c("#116329"),
	},
	Dark: Palette{
		Accent: c("5"), Added: c("2"), Removed: c("1"), Meta: c("6"), Warn: c("3"),
		Muted: c("#8b949e"), RowBg: c("#21262d"), Border: c("#30363d"), Select: c("#58a6ff"),
		Canvas: nil, Text: nil, OverlayBg: c("#161b22"),
		AddBg: c("#003914"), AddGut: c("#00672c"), AddFg: c("#7fe998"),
		DelBg: c("#5d0003"), DelGut: c("#a4000d"), DelFg: c("#ffa598"),
		HunkBg: c("#002c60"), HunkFg: c("#88d1ff"),
		FillBg: c("#0d1117"), ExpandBg: c("#12202f"),
		SearchBg: c("#4a3a00"), SearchCurBg: c("#9e6a03"), Tag: c("#d4a72c"),
		Pulse:      []color.Color{c("#233447"), c("#304b67"), c("#3d6389"), c("#4a7cad"), c("#5895d3"), c("#67b0f9")},
		SynComment: c("#8b949e"), SynString: c("#a5d6ff"), SynNumber: c("#79c0ff"),
		SynKeyword: c("#ff7b72"), SynType: c("#ffa657"), SynFunction: c("#d2a8ff"), SynTag: c("#7ee787"),
	},
}

// src is the compact set of source hues a derived theme supplies; paletteFrom
// expands it into a full Palette, deriving the diff tints, search bands, fill and
// overlay tones from the canvas + accent colors.
type src struct {
	dark                                         bool
	bg, fg, muted, rowBg, border                 color.Color
	accent, sel, green, red, meta, warn, tag     color.Color
	synComment, synString, synNumber, synKeyword color.Color
	synType, synFunc, synTag                     color.Color
}

// paletteFrom derives a full Palette from a handful of source hues. Diff bands
// are the hue blended toward the canvas (a deep body band, a brighter gutter
// band); code/marker foregrounds are the hue pushed toward white (dark) or black
// (light) so they stay legible on their band.
func paletteFrom(s src) Palette {
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	black := color.RGBA{0x10, 0x10, 0x10, 0xff}

	var bgF, gutF, fgF, searchBgF, searchCurF float64
	if s.dark {
		bgF, gutF, fgF, searchBgF, searchCurF = 0.80, 0.55, 0.45, 0.80, 0.52
	} else {
		bgF, gutF, fgF, searchBgF, searchCurF = 0.86, 0.62, 0.42, 0.82, 0.58
	}
	ink := func(hue color.Color, amt float64) color.Color {
		if s.dark {
			return blendColor(hue, white, amt)
		}
		return blendColor(hue, black, amt)
	}
	tone := func(hue color.Color) (bg, gut, fg color.Color) {
		return blendColor(hue, s.bg, bgF), blendColor(hue, s.bg, gutF), ink(hue, fgF)
	}

	addBg, addGut, addFg := tone(s.green)
	delBg, delGut, delFg := tone(s.red)
	hunkBg, _, _ := tone(s.meta)

	return Palette{
		Accent: s.accent, Added: s.green, Removed: s.red, Meta: s.meta, Warn: s.warn,
		Muted: s.muted, RowBg: s.rowBg, Border: s.border, Select: s.sel,
		Canvas: s.bg, Text: s.fg, OverlayBg: blendColor(s.bg, s.fg, 0.07),
		AddBg: addBg, AddGut: addGut, AddFg: addFg,
		DelBg: delBg, DelGut: delGut, DelFg: delFg,
		HunkBg: hunkBg, HunkFg: ink(s.meta, 0.40),
		FillBg:   blendColor(s.bg, s.border, 0.25),
		ExpandBg: blendColor(s.meta, s.bg, 0.90),
		SearchBg: blendColor(s.warn, s.bg, searchBgF), SearchCurBg: blendColor(s.warn, s.bg, searchCurF),
		Tag:        s.tag,
		SynComment: s.synComment, SynString: s.synString, SynNumber: s.synNumber,
		SynKeyword: s.synKeyword, SynType: s.synType, SynFunction: s.synFunc, SynTag: s.synTag,
	}
}

// --- Dracula ---

var themeDracula = Theme{
	Name: "Dracula",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#282a36"), fg: c("#f8f8f2"), muted: c("#6272a4"), rowBg: c("#44475a"), border: c("#3a3d4d"),
		accent: c("#ff79c6"), sel: c("#bd93f9"), green: c("#50fa7b"), red: c("#ff5555"), meta: c("#8be9fd"), warn: c("#f1fa8c"), tag: c("#f1fa8c"),
		synComment: c("#6272a4"), synString: c("#f1fa8c"), synNumber: c("#bd93f9"), synKeyword: c("#ff79c6"),
		synType: c("#8be9fd"), synFunc: c("#50fa7b"), synTag: c("#ff79c6"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#f8f8f2"), fg: c("#1f1f28"), muted: c("#7c7a99"), rowBg: c("#e6e3f0"), border: c("#d6d2e6"),
		accent: c("#cc4ea3"), sel: c("#7d4fd1"), green: c("#1f9b4e"), red: c("#d23c3c"), meta: c("#1c8aa8"), warn: c("#9a7a00"), tag: c("#9a7a00"),
		synComment: c("#8a87a8"), synString: c("#6f6000"), synNumber: c("#7d4fd1"), synKeyword: c("#cc4ea3"),
		synType: c("#1c8aa8"), synFunc: c("#1f9b4e"), synTag: c("#cc4ea3"),
	}),
}

// --- Nord ---

var themeNord = Theme{
	Name: "Nord",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#2e3440"), fg: c("#d8dee9"), muted: c("#616e88"), rowBg: c("#3b4252"), border: c("#434c5e"),
		accent: c("#88c0d0"), sel: c("#81a1c1"), green: c("#a3be8c"), red: c("#bf616a"), meta: c("#88c0d0"), warn: c("#ebcb8b"), tag: c("#ebcb8b"),
		synComment: c("#616e88"), synString: c("#a3be8c"), synNumber: c("#b48ead"), synKeyword: c("#81a1c1"),
		synType: c("#8fbcbb"), synFunc: c("#88c0d0"), synTag: c("#8fbcbb"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#eceff4"), fg: c("#2e3440"), muted: c("#7b8794"), rowBg: c("#dde1e9"), border: c("#d2d6de"),
		accent: c("#5e81ac"), sel: c("#5e81ac"), green: c("#6a924a"), red: c("#bf616a"), meta: c("#2c7a86"), warn: c("#b48a2e"), tag: c("#b48a2e"),
		synComment: c("#8a93a3"), synString: c("#4f6b34"), synNumber: c("#9d6fa0"), synKeyword: c("#5e81ac"),
		synType: c("#2c7a86"), synFunc: c("#4d7ea8"), synTag: c("#2c7a86"),
	}),
}

// --- Catppuccin (Mocha / Latte) ---

var themeCatppuccin = Theme{
	Name: "Catppuccin",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#1e1e2e"), fg: c("#cdd6f4"), muted: c("#6c7086"), rowBg: c("#313244"), border: c("#45475a"),
		accent: c("#cba6f7"), sel: c("#89b4fa"), green: c("#a6e3a1"), red: c("#f38ba8"), meta: c("#89dceb"), warn: c("#f9e2af"), tag: c("#f9e2af"),
		synComment: c("#6c7086"), synString: c("#a6e3a1"), synNumber: c("#fab387"), synKeyword: c("#cba6f7"),
		synType: c("#f9e2af"), synFunc: c("#89b4fa"), synTag: c("#f38ba8"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#eff1f5"), fg: c("#4c4f69"), muted: c("#8c8fa1"), rowBg: c("#dce0e8"), border: c("#ccd0da"),
		accent: c("#8839ef"), sel: c("#1e66f5"), green: c("#40a02b"), red: c("#d20f39"), meta: c("#04a5e5"), warn: c("#df8e1d"), tag: c("#df8e1d"),
		synComment: c("#8c8fa1"), synString: c("#40a02b"), synNumber: c("#fe640b"), synKeyword: c("#8839ef"),
		synType: c("#df8e1d"), synFunc: c("#1e66f5"), synTag: c("#d20f39"),
	}),
}

// --- Gruvbox ---

var themeGruvbox = Theme{
	Name: "Gruvbox",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#282828"), fg: c("#ebdbb2"), muted: c("#928374"), rowBg: c("#3c3836"), border: c("#504945"),
		accent: c("#d3869b"), sel: c("#83a598"), green: c("#b8bb26"), red: c("#fb4934"), meta: c("#8ec07c"), warn: c("#fabd2f"), tag: c("#fabd2f"),
		synComment: c("#928374"), synString: c("#b8bb26"), synNumber: c("#d3869b"), synKeyword: c("#fb4934"),
		synType: c("#fabd2f"), synFunc: c("#8ec07c"), synTag: c("#8ec07c"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#fbf1c7"), fg: c("#3c3836"), muted: c("#7c6f64"), rowBg: c("#ebdbb2"), border: c("#d5c4a1"),
		accent: c("#b16286"), sel: c("#076678"), green: c("#79740e"), red: c("#9d0006"), meta: c("#427b58"), warn: c("#b57614"), tag: c("#b57614"),
		synComment: c("#928374"), synString: c("#79740e"), synNumber: c("#8f3f71"), synKeyword: c("#9d0006"),
		synType: c("#b57614"), synFunc: c("#427b58"), synTag: c("#427b58"),
	}),
}

// --- Tokyo Night (Night / Day) ---

var themeTokyoNight = Theme{
	Name: "Tokyo Night",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#1a1b26"), fg: c("#c0caf5"), muted: c("#565f89"), rowBg: c("#292e42"), border: c("#3b4261"),
		accent: c("#bb9af7"), sel: c("#7aa2f7"), green: c("#9ece6a"), red: c("#f7768e"), meta: c("#7dcfff"), warn: c("#e0af68"), tag: c("#e0af68"),
		synComment: c("#565f89"), synString: c("#9ece6a"), synNumber: c("#ff9e64"), synKeyword: c("#bb9af7"),
		synType: c("#2ac3de"), synFunc: c("#7aa2f7"), synTag: c("#f7768e"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#e1e2e7"), fg: c("#343b58"), muted: c("#848cb5"), rowBg: c("#d0d2db"), border: c("#c4c8da"),
		accent: c("#9854f1"), sel: c("#2e7de9"), green: c("#587539"), red: c("#f52a65"), meta: c("#007197"), warn: c("#8c6c3e"), tag: c("#8c6c3e"),
		synComment: c("#848cb5"), synString: c("#587539"), synNumber: c("#b15c00"), synKeyword: c("#9854f1"),
		synType: c("#007197"), synFunc: c("#2e7de9"), synTag: c("#f52a65"),
	}),
}

// --- Solarized ---

var themeSolarized = Theme{
	Name: "Solarized",
	Dark: paletteFrom(src{
		dark: true,
		bg:   c("#002b36"), fg: c("#839496"), muted: c("#586e75"), rowBg: c("#073642"), border: c("#0a4b5a"),
		accent: c("#d33682"), sel: c("#268bd2"), green: c("#859900"), red: c("#dc322f"), meta: c("#2aa198"), warn: c("#b58900"), tag: c("#b58900"),
		synComment: c("#586e75"), synString: c("#2aa198"), synNumber: c("#d33682"), synKeyword: c("#859900"),
		synType: c("#b58900"), synFunc: c("#268bd2"), synTag: c("#cb4b16"),
	}),
	Light: paletteFrom(src{
		dark: false,
		bg:   c("#fdf6e3"), fg: c("#657b83"), muted: c("#93a1a1"), rowBg: c("#eee8d5"), border: c("#ddd6c1"),
		accent: c("#d33682"), sel: c("#268bd2"), green: c("#859900"), red: c("#dc322f"), meta: c("#2aa198"), warn: c("#b58900"), tag: c("#b58900"),
		synComment: c("#93a1a1"), synString: c("#2aa198"), synNumber: c("#d33682"), synKeyword: c("#859900"),
		synType: c("#b58900"), synFunc: c("#268bd2"), synTag: c("#cb4b16"),
	}),
}

// --- Monochrome (NO_COLOR target) — no color at all; structure carries the UI ---

var monoPalette = Palette{
	// Every slot nil: foregrounds follow the terminal, backgrounds are absent, so
	// the UI reads through bold weight, the ▸/+/- glyphs, and layout alone.
}

var themeMonochrome = Theme{
	Name:  "Monochrome",
	Light: monoPalette,
	Dark:  monoPalette,
}

package tui

import (
	"path/filepath"
	"strings"
)

// icons.go renders the optional file-type icon shown before a file's name in the
// tree. It has three tiers: ascii (no icon — the original layout), unicode (a
// single neutral bullet, safe on any terminal), and nerd (Nerd Font devicons by
// file type, which need a patched font but degrade to a placeholder box if it's
// absent). Folders keep their ▸/▾ chevron and get no extra icon.

type iconSet int

const (
	iconASCII iconSet = iota
	iconUnicode
	iconNerd
)

// iconSetNames maps each tier to its config/flag token, in registry order.
var iconSetNames = []string{"ascii", "unicode", "nerd"}

func (s iconSet) String() string {
	if int(s) >= 0 && int(s) < len(iconSetNames) {
		return iconSetNames[int(s)]
	}
	return "ascii"
}

// parseIconSet resolves a tier name (case-insensitive) to an iconSet, falling
// back to ascii with ok=false when unknown. "nerdfont" is accepted as an alias.
func parseIconSet(name string) (iconSet, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ascii", "none", "":
		return iconASCII, name == "ascii" || name == "none"
	case "unicode":
		return iconUnicode, true
	case "nerd", "nerdfont", "nerd-font":
		return iconNerd, true
	}
	return iconASCII, false
}

// fileIcon returns the type icon for a file path under the given tier, or "" when
// no icon should be shown (the ascii tier, always). The caller adds the trailing
// space and accounts for the icon's display width.
func fileIcon(path string, set iconSet) string {
	switch set {
	case iconUnicode:
		return "·"
	case iconNerd:
		return nerdIcon(path)
	default:
		return ""
	}
}

// nerdIcon maps a file to a Nerd Font devicon by base name first (Dockerfile,
// Makefile, lockfiles) then by extension, with a generic file glyph as the
// fallback. Codepoints are in the Nerd Font private-use range; a terminal
// without a patched font renders them as a placeholder, which is why the tier is
// opt-in.
func nerdIcon(path string) string {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return ""
	case "makefile":
		return ""
	case "go.mod", "go.sum":
		return ""
	case "package.json", "package-lock.json":
		return ""
	case "license", "license.md", "license.txt":
		return ""
	case ".gitignore", ".gitattributes":
		return ""
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return ""
	case ".js", ".cjs", ".mjs":
		return ""
	case ".ts", ".tsx":
		return ""
	case ".jsx":
		return ""
	case ".py":
		return ""
	case ".rs":
		return ""
	case ".rb":
		return ""
	case ".java":
		return ""
	case ".c", ".h":
		return ""
	case ".cpp", ".cc", ".cxx", ".hpp":
		return ""
	case ".cs":
		return ""
	case ".php":
		return ""
	case ".swift":
		return ""
	case ".kt", ".kts":
		return ""
	case ".html", ".htm":
		return ""
	case ".css":
		return ""
	case ".scss", ".sass":
		return ""
	case ".json":
		return ""
	case ".md", ".markdown":
		return ""
	case ".sh", ".bash", ".zsh":
		return ""
	case ".yml", ".yaml":
		return ""
	case ".toml", ".ini", ".cfg", ".conf":
		return ""
	case ".lock":
		return ""
	case ".sql":
		return ""
	case ".vue":
		return ""
	case ".lua":
		return ""
	case ".dart":
		return ""
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico":
		return ""
	case ".txt", ".text":
		return ""
	}
	return "" // generic file
}

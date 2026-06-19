package tui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
)

// config.go handles persisted preferences and resolves the effective startup
// options from (in precedence order) command-line flags, environment variables,
// the on-disk config file, and finally terminal auto-detection. NO_COLOR, when
// present, overrides everything with the monochrome theme and no animation.

// userConfig is the persisted preference file. Dark is a pointer so "unset"
// (auto-detect from the terminal) is distinct from an explicit light choice.
type userConfig struct {
	Theme        string `json:"theme,omitempty"`
	Icons        string `json:"icons,omitempty"`
	ReduceMotion bool   `json:"reduceMotion,omitempty"`
	Dark         *bool  `json:"dark,omitempty"`
}

// Options carries the raw command-line flag values into Run; empty strings mean
// "flag not given". NoAnimSet distinguishes an explicit --no-anim=false from the
// flag being absent.
type Options struct {
	Theme     string
	Icons     string
	NoAnim    bool
	NoAnimSet bool
}

// resolved is the effective configuration after applying precedence.
type resolved struct {
	themeIdx     int
	iconSet      iconSet
	reduceMotion bool
	dark         bool
}

// configPath returns the preference file location, following the XDG Base
// Directory spec: $XDG_CONFIG_HOME/diffcat/config.json, falling back to
// ~/.config/diffcat/config.json. This is used on every platform (unlike
// os.UserConfigDir, which resolves to ~/Library/Application Support on macOS)
// so the documented ~/.config path holds everywhere.
func configPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "diffcat", "config.json"), nil
}

// loadConfig reads the preference file, returning a zero config on any error
// (missing file, unreadable, malformed) — preferences are strictly best-effort.
func loadConfig() userConfig {
	var cfg userConfig
	p, err := configPath()
	if err != nil {
		return cfg
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

// saveConfig writes the preference file, creating the directory if needed.
func saveConfig(cfg userConfig) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// resolveOptions applies the precedence chain flag > env > config > detect to
// produce the effective startup configuration.
func resolveOptions(o Options, cfg userConfig) resolved {
	// NO_COLOR (no-color.org): when present, force monochrome and freeze motion,
	// overriding every other source.
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		idx := themeIndexByName("Monochrome")
		if idx < 0 {
			idx = 0
		}
		return resolved{themeIdx: idx, iconSet: iconASCII, reduceMotion: true, dark: detectDark()}
	}

	r := resolved{}

	name := firstNonEmpty(o.Theme, os.Getenv("DIFFCAT_THEME"), cfg.Theme)
	if idx := themeIndexByName(name); idx >= 0 {
		r.themeIdx = idx
	}

	r.iconSet, _ = parseIconSet(firstNonEmpty(o.Icons, os.Getenv("DIFFCAT_ICONS"), cfg.Icons))

	switch {
	case o.NoAnimSet:
		r.reduceMotion = o.NoAnim
	case os.Getenv("DIFFCAT_NO_ANIM") != "":
		r.reduceMotion = true
	default:
		r.reduceMotion = cfg.ReduceMotion
	}

	if cfg.Dark != nil {
		r.dark = *cfg.Dark
	} else {
		r.dark = detectDark()
	}
	return r
}

func detectDark() bool { return lipgloss.HasDarkBackground(os.Stdin, os.Stdout) }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

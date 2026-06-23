package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// state.go persists per-session UI state that isn't a preference: whether the
// one-time onboarding hint has been shown. Unlike config.go (preferences the
// reader chose), this is progress the tool remembers on the reader's behalf. It
// lives under the XDG state dir (transient, machine-local) rather than the config
// dir.

// userState is the persisted progress file.
type userState struct {
	FirstRunDone bool `json:"firstRunDone,omitempty"`
}

// statePath returns the progress file location, following the XDG Base Directory
// spec: $XDG_STATE_HOME/diffcat/state.json, falling back to
// ~/.local/state/diffcat/state.json. Used on every platform for a documented,
// predictable path (mirrors configPath's reasoning).
func statePath() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "diffcat", "state.json"), nil
}

// loadState reads the progress file, returning a zero state on any error
// (missing, unreadable, malformed) — progress is strictly best-effort.
func loadState() userState {
	var st userState
	p, err := statePath()
	if err != nil {
		return st
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

// saveState writes the progress file, creating the directory if needed.
func saveState(st userState) error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// markFirstRunDone records that the one-time onboarding hint has been shown, so
// it never appears again. Best-effort.
func markFirstRunDone(repo string) {
	st := loadState()
	st.FirstRunDone = true
	_ = saveState(st)
}

package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// state.go persists per-session UI state that isn't a preference: which files /
// commits the reader has marked reviewed, and whether the one-time onboarding
// hint has been shown. Unlike config.go (preferences the reader chose), this is
// progress the tool remembers on the reader's behalf. It lives under the XDG
// state dir (transient, machine-local) rather than the config dir.

// reviewScope is the set of reviewed item ids (file paths in the branch/commit
// views, commit SHAs in the history view) for one diff snapshot. Key is the
// base..HEAD signature the marks were made against; when it changes (new commits
// or a different base) the marks no longer map onto the diff and are dropped, so
// stale checkmarks expire on their own without a manual reset.
type reviewScope struct {
	Key   string          `json:"key"`
	Items map[string]bool `json:"items,omitempty"`
}

// userState is the persisted progress file. Reviews is keyed by repo root so one
// state file can track several checkouts.
type userState struct {
	FirstRunDone bool                   `json:"firstRunDone,omitempty"`
	Reviews      map[string]reviewScope `json:"reviews,omitempty"`
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

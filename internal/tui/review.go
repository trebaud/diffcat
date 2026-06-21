package tui

import "github.com/trebaud/diffcat/internal/git"

// review.go is the review-progress hook: marking files (branch/commit views) or
// commits (history view) as read, tracking how much of the diff is cleared, and
// firing the celebration when the last one is marked. Rendering (the header
// progress bar, the row checkmark, the celebration card) lives in view.go /
// onboard.go; persistence in state.go.

// celebrateFrames is how many animation frames the "branch cleared" celebration
// holds before auto-dismissing (~1.4s at the slow tick).
const celebrateFrames = 28

// reviewKey is the signature the current marks are scoped to: the base the diff
// is measured against and the commit at HEAD. When either moves the key changes,
// so marks made against the old diff are dropped on the next load.
func (m model) reviewKey() string {
	return m.base + ".." + git.HeadSHA(m.repo)
}

// reviewID returns the id of the reviewable item under the cursor in the current
// view — a file path in the branch/commit trees, a commit SHA in the history
// list — or "" when the cursor is on nothing reviewable (a folder, the working
// row, an empty list, or a mode without a review hook).
func (m model) reviewID() string {
	switch m.mode {
	case viewBranch, viewCommit:
		if f := m.selectedFile(); f != nil {
			return f.Path
		}
	case viewLog:
		if !m.onWorkingRow() {
			if c := m.selectedCommit(); c != nil {
				return c.SHA
			}
		}
	}
	return ""
}

// reviewScopeIDs lists every reviewable id in the current view, in display order
// — the basis for the progress totals and the next-unreviewed jump. The history
// view counts only the branch's own commits (above the base divider), matching
// what the header's commit count reports.
func (m model) reviewScopeIDs() []string {
	switch m.mode {
	case viewBranch, viewCommit:
		ids := make([]string, 0, len(m.files))
		for _, f := range m.files {
			ids = append(ids, f.Path)
		}
		return ids
	case viewLog:
		n := m.featureCommitCount()
		ids := make([]string, 0, n)
		for i := 0; i < n && i < len(m.commits); i++ {
			ids = append(ids, m.commits[i].SHA)
		}
		return ids
	}
	return nil
}

// reviewProgress returns how many of the current view's reviewable items are
// marked and the total. A zero total means the view has no review hook (or
// nothing to review), and callers hide the progress bar.
func (m model) reviewProgress() (done, total int) {
	ids := m.reviewScopeIDs()
	for _, id := range ids {
		if m.reviewed[id] {
			done++
		}
	}
	return done, len(ids)
}

// toggleReviewed flips the reviewed flag on the item under the cursor. Marking
// the final item clears the diff and fires the celebration (once); marking any
// earlier item hops the cursor to the next still-unreviewed item, so clearing a
// branch flows without reaching for a navigation key. Unmarking re-arms the
// celebration so a re-clear can fire it again. Every change is persisted.
func (m *model) toggleReviewed() {
	id := m.reviewID()
	if id == "" {
		return
	}
	if m.reviewed == nil {
		m.reviewed = map[string]bool{}
	}
	if m.reviewed[id] {
		delete(m.reviewed, id)
		m.celebrated = false
	} else {
		m.reviewed[id] = true
		if done, total := m.reviewProgress(); total > 0 && done == total {
			if !m.celebrated {
				m.celebrate = celebrateFrames
				m.celebrated = true
			}
		} else {
			m.advanceToNextUnreviewed()
		}
	}
	m.persistReviews()
}

// advanceToNextUnreviewed moves the cursor to the next still-unreviewed item
// after the current one, wrapping around. It's the "keep clearing" motor behind
// toggleReviewed (and the palette's "next unreviewed" action).
func (m *model) advanceToNextUnreviewed() {
	switch m.mode {
	case viewBranch, viewCommit:
		n := len(m.rows)
		if n == 0 {
			return
		}
		for off := 1; off <= n; off++ {
			i := (m.cursor + off) % n
			if r := m.rows[i]; r.file != nil && !m.reviewed[r.file.Path] {
				m.cursor = i
				m.loadDiff()
				return
			}
		}
	case viewLog:
		total := m.logRowCount()
		if total == 0 {
			return
		}
		for off := 1; off <= total; off++ {
			idx := (m.commitCursor + off) % total
			ci := idx
			if m.logWorking {
				ci--
			}
			if ci >= 0 && ci < m.featureCommitCount() && ci < len(m.commits) && !m.reviewed[m.commits[ci].SHA] {
				m.commitCursor = idx
				if m.logDiffOpen {
					m.loadCommitDiff()
				}
				return
			}
		}
	}
}

// persistReviews writes the current marks back to the state file, scoped to this
// repo and the current base..HEAD key. It reloads first so a concurrent change
// to another repo's marks (or the first-run flag) isn't clobbered. Best-effort —
// a write failure leaves the in-session marks standing.
func (m model) persistReviews() {
	st := loadState()
	if st.Reviews == nil {
		st.Reviews = map[string]reviewScope{}
	}
	items := make(map[string]bool, len(m.reviewed))
	for id, done := range m.reviewed {
		if done {
			items[id] = true
		}
	}
	st.Reviews[m.repo] = reviewScope{Key: m.reviewKey(), Items: items}
	_ = saveState(st)
}

// markFirstRunDone records that the one-time onboarding hint has been shown, so
// it never appears again. Best-effort.
func markFirstRunDone(repo string) {
	st := loadState()
	st.FirstRunDone = true
	_ = saveState(st)
}

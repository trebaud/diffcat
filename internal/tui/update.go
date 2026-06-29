package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// tickMsg drives the nyan cat. The loop runs at tickSlow when the cat is settled
// — ~7fps, enough for a charming gait without busy-spinning the terminal — and
// speeds up to tickFast only while the spring is in motion, so a glide reads as
// smooth without paying the redraw cost when nothing's moving.
type tickMsg struct{}

const (
	tickSlow = 150 * time.Millisecond // settled: gait/pulse/shimmer only
	tickFast = 33 * time.Millisecond  // ~30fps while the position spring is moving

	// settleFrames is how many fast ticks the cat's arrival blink lasts (~800ms).
	settleFrames = 24
)

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// gitStateMsg carries the result of a background git-state poll. The fingerprint
// is computed in the tick goroutine (off the UI thread), so the poll never
// blocks rendering even on a slow repo.
type gitStateMsg struct{ fingerprint string }

// syncInterval is how long the poll waits between fingerprint checks. It
// self-paces: the next poll is scheduled only after the previous one returns
// (Update re-arms it), so a slow `git status` can't pile up overlapping polls.
const syncInterval = 1500 * time.Millisecond

func syncCmd(repo, baseName string) tea.Cmd {
	return tea.Tick(syncInterval, func(time.Time) tea.Msg {
		return gitStateMsg{fingerprint: git.Fingerprint(repo, baseName)}
	})
}

// startupMsg carries the result of the background launch gather — the change
// list, history, status fingerprint, and working count — computed off the render
// path so the first frame can paint a loading screen immediately. applyStartup
// seeds the model from it and drops out of the loading state.
type startupMsg struct{ su startup }

// startupCmd runs the heavy launch git work in the background (see gatherStartup),
// landing as a startupMsg. Kicked from Init.
func startupCmd(repo, base, baseName string) tea.Cmd {
	return func() tea.Msg { return startupMsg{su: gatherStartup(repo, base, baseName)} }
}

// fullHistoryMsg carries the complete (uncapped) commit history walked in the
// background after the capped launch list painted, to be swapped in by Update.
type fullHistoryMsg struct {
	commits   []git.Commit
	baseStart int
}

// fullHistoryCmd walks the full HEAD-side history (uncapped) off the UI thread —
// the backfill for the capped launch list. Kicked from applyStartup only when the
// capped walk was actually truncated, so a short branch never pays for it.
func fullHistoryCmd(repo, base string) tea.Cmd {
	return func() tea.Msg {
		cs, start, _ := git.BranchHistory(repo, base, baseHistoryLimit, 0)
		return fullHistoryMsg{commits: cs, baseStart: start}
	}
}

// historyMsg carries the result of a background whole-history computation. It
// shells `git log --numstat` over every commit reachable from HEAD, which can
// take a second or two on a deep history — so it runs off the UI thread and lands
// here rather than blocking the Summary dashboard open.
type historyMsg struct{ stats git.HistoryStats }

// historyCmd computes the whole-history Summary stats in the background.
func historyCmd(repo string) tea.Cmd {
	return func() tea.Msg { return historyMsg{stats: git.History(repo, "HEAD")} }
}

// ensureHistory kicks the background whole-history computation when it isn't
// already computed or in flight, returning the command to run (nil when there's
// nothing to do). It records the HEAD the run is keyed to so refresh can tell
// whether the cache is stale. The result arrives as a historyMsg.
func (m *model) ensureHistory() tea.Cmd {
	if m.historyComputed || m.historyComputing {
		return nil
	}
	m.historyComputing = true
	m.historyHead = git.HeadSHA(m.repo)
	return historyCmd(m.repo)
}

// authorModulesMsg carries one contributor's lazily-computed module ranking back to
// the UI thread; author keys it into the cache.
type authorModulesMsg struct {
	author  string
	modules []git.ModuleCount
}

// authorModulesCmd computes a contributor's module ranking in the background by
// diffing only their commits (git.AuthorModules), so the heavy numstat work never
// touches the dashboard's whole-repo open path.
func authorModulesCmd(repo, author string, shas []string) tea.Cmd {
	return func() tea.Msg {
		return authorModulesMsg{author: author, modules: git.AuthorModules(repo, shas)}
	}
}

// ensureAuthorModules kicks the lazy module computation for one contributor the first
// time their page opens, returning the command (nil when already cached, in flight,
// or there's nothing to diff). The result arrives as an authorModulesMsg.
func (m *model) ensureAuthorModules(name string) tea.Cmd {
	if name == "" {
		return nil
	}
	if _, done := m.authorModules[name]; done {
		return nil
	}
	if m.authorModulesComputing[name] {
		return nil
	}
	shas := m.historyStats.AuthorSHAs[name]
	if len(shas) == 0 {
		return nil
	}
	m.authorModulesComputing[name] = true
	return authorModulesCmd(m.repo, name, shas)
}

// Update is the Elm update function — it maps a message to the next model and
// any side effects.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampDiffOffset()
		m.clampDiffCursor()
		m.ensureCursorVisible()
		return m, nil

	case startupMsg:
		// The background launch gather landed: seed the model and drop the loading
		// screen. A WindowSizeMsg has almost certainly already set width/height, so
		// the next render shows the populated view immediately. applyStartup returns
		// a backfill command when the capped history list has more commits to load.
		return m, m.applyStartup(msg.su)

	case fullHistoryMsg:
		// The background full-history walk completed: swap the capped launch list for
		// the complete one, holding the reader's place by SHA (and on the working-tree
		// row when present). Safe in any mode — m.commits isn't repurposed by the
		// drill-in, and the diff cache is left intact (no re-parse). It reflects the
		// same git state the launch list did, so commitListSynced is unchanged.
		wasWorking := m.onWorkingRow()
		prevSHA := ""
		if c := m.selectedCommit(); c != nil {
			prevSHA = c.SHA
		}
		m.commits = msg.commits
		m.baseStart = msg.baseStart
		if wasWorking && m.logWorking {
			m.commitCursor = 0
		} else {
			m.reselectCommit(prevSHA)
		}
		return m, nil

	case tickMsg:
		// Reduce-motion freezes every animation: let the loop die (it re-arms only
		// when motion is toggled back on, guarded by m.ticking).
		if m.reduceMotion {
			m.ticking = false
			return m, nil
		}
		// Spring the cat's position toward the cursor. A soft underdamped spring
		// (low ω for a longer, smoother catch-up; ζ≈0.5 for a pronounced bounce) —
		// on a jump it glides through more intermediate cells and visibly overshoots
		// (~16%) before springing back, and line-by-line moves trail smoothly rather
		// than snapping. Integrated with a fixed dt for stability; when settled
		// target≈pos, so this is a no-op regardless of the (then slower) tick rate.
		target := m.targetFrac()
		if m.focus != focusDiff {
			m.nyanPos, m.nyanVel = target, 0 // hidden: snap so it's in place when focused
		} else {
			const omega, zeta, dt = 11.0, 0.5, 0.033
			a := omega*omega*(target-m.nyanPos) - 2*zeta*omega*m.nyanVel
			m.nyanVel += a * dt
			m.nyanPos += m.nyanVel * dt
		}
		// The instant the spring settles after a glide, kick off a one-shot "settle"
		// reaction (a happy blink) — distinct from the perpetual leg gait. Detected
		// as the moving→stopped edge while the cat is on screen.
		moving := m.focus == focusDiff && m.nyanMoving(target)
		if m.nyanWasMoving && !moving {
			m.nyanSettle = settleFrames
		}
		m.nyanWasMoving = moving
		if m.nyanSettle > 0 {
			m.nyanSettle--
		}
		// Pace animFrame (gait/pulse/shimmer) on wall-clock so its ~7fps look is
		// unchanged whether the physics loop is running fast or slow.
		interval := tickSlow
		if moving || m.nyanSettle > 0 { // run fast through the glide and its settle blink
			interval = tickFast
		}
		m.animAccum += interval
		for m.animAccum >= tickSlow {
			m.animFrame++
			m.animAccum -= tickSlow
		}
		// Retire the onboarding flourish on its own timer.
		if m.showToast && m.animFrame-m.toastStart >= toastFrames {
			m.showToast = false
		}
		return m, tickCmd(interval)

	case historyMsg:
		m.historyStats = msg.stats
		m.historyComputed = true
		m.historyComputing = false
		// A live HEAD move while a contributor's detail page is open invalidates
		// the whole-history cache (refresh drops historyComputed and the lazy
		// authorModules), so this recompute is the first point the fresh
		// AuthorSHAs exist. Re-kick the open author's module ranking against them
		// so the heatmap reappears updated rather than staying blank until the
		// page is reopened.
		if m.mode == viewAuthorDetail {
			return m, m.ensureAuthorModules(m.detailAuthor)
		}
		return m, nil

	case authorModulesMsg:
		// Cache the lazily-computed ranking regardless of the current view (the reader
		// may have navigated away). A present key — even an empty ranking — marks it
		// done so it isn't recomputed.
		if m.authorModules == nil {
			m.authorModules = map[string][]git.ModuleCount{}
		}
		m.authorModules[msg.author] = msg.modules
		delete(m.authorModulesComputing, msg.author)
		return m, nil

	case gitStateMsg:
		// While the initial gather is still in flight the model has no data to
		// refresh against (and applyStartup will seed the fingerprint baseline), so
		// just keep the poll alive without firing a spurious refresh.
		if m.loading {
			return m, syncCmd(m.repo, m.baseName)
		}
		// Only do the (more expensive) refresh when the cheap fingerprint moved;
		// otherwise the poll is a no-op beyond re-arming itself.
		var repaint tea.Cmd
		if msg.fingerprint != m.syncFingerprint {
			m.syncFingerprint = msg.fingerprint
			repaint = m.refresh()
		}
		sync := syncCmd(m.repo, m.baseName)
		// If the commit that moved invalidated the whole-history stats and the Stats
		// dashboard (or a contributor's detail page) is on screen, recompute them
		// live; otherwise leave it for the next open. (refresh only invalidates when
		// HEAD actually moved.)
		if m.mode == viewOverview || m.mode == viewAuthorDetail {
			return m, tea.Batch(sync, repaint, m.ensureHistory())
		}
		return m, tea.Batch(sync, repaint)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While the initial gather is still loading there's nothing to act on yet —
	// only honor quit so the reader is never trapped on the loading screen.
	if m.loading {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Once the reader touches a key, retire the first-run hint (it also auto-fades);
	// the keypress itself still goes on to act.
	m.showToast = false

	if m.showGlobalSearch {
		// Global search is open. Enter navigates to the highlighted hit; esc (or
		// ctrl+k again) closes; arrows / ctrl+n/p move; the rest edits the query.
		results := m.gsResults()
		switch msg.String() {
		case "esc", "ctrl+k":
			m.showGlobalSearch = false
		case "enter":
			m.showGlobalSearch = false
			if m.gsSel >= 0 && m.gsSel < len(results) {
				return m, m.gsActivate(results[m.gsSel])
			}
		case "down", "ctrl+n":
			if m.gsSel < len(results)-1 {
				m.gsSel++
			}
		case "up", "ctrl+p":
			if m.gsSel > 0 {
				m.gsSel--
			}
		case "right", "pgdown", "ctrl+f":
			// Next page: select its first result (the page is derived from the
			// selection, so this flips the view). Clamped to the last result.
			if len(results) > 0 {
				ps := m.gsPageSize()
				page := m.gsSel / ps
				m.gsSel = min((page+1)*ps, len(results)-1)
			}
		case "left", "pgup", "ctrl+b":
			// Previous page: select its first result.
			ps := m.gsPageSize()
			if page := m.gsSel / ps; page > 0 {
				m.gsSel = (page - 1) * ps
			} else {
				m.gsSel = 0
			}
		case "backspace":
			if r := []rune(m.gsInput); len(r) > 0 {
				m.gsInput = string(r[:len(r)-1])
			}
			m.gsSel = 0
		default:
			// msg.Text carries the actual typed characters (a space arrives as the key
			// name "space", so a String()-length check would drop it); it's empty for
			// non-text keys like the arrows, which the cases above already handle.
			if msg.Text != "" {
				m.gsInput += msg.Text
				m.gsSel = 0
			}
		}
		return m, nil
	}

	if m.showPalette {
		// The command palette is open. Enter runs the highlighted action; esc (or
		// `:` again) closes; arrows / ctrl+n/p move; the rest edits the query.
		actions := m.paletteMatches()
		switch msg.String() {
		case "esc", ":":
			m.showPalette = false
		case "enter":
			m.showPalette = false
			if m.paletteSel >= 0 && m.paletteSel < len(actions) {
				return m, actions[m.paletteSel].action.do(&m)
			}
		case "down", "ctrl+n":
			if m.paletteSel < len(actions)-1 {
				m.paletteSel++
			}
		case "up", "ctrl+p":
			if m.paletteSel > 0 {
				m.paletteSel--
			}
		case "backspace":
			if r := []rune(m.paletteInput); len(r) > 0 {
				m.paletteInput = string(r[:len(r)-1])
			}
			m.paletteSel = 0
		default:
			if s := msg.String(); len([]rune(s)) == 1 {
				m.paletteInput += s
				m.paletteSel = 0
			}
		}
		return m, nil
	}

	if m.showHelp {
		// Any key dismisses the help overlay.
		m.showHelp = false
		m.pendingG = false
		return m, nil
	}

	if m.showCommitDetails {
		// j/k (and half/full page) scroll a long body; any other key dismisses.
		switch msg.String() {
		case "j", "down":
			m.detailsScroll++
		case "k", "up":
			m.detailsScroll--
		case "ctrl+d", "ctrl+f":
			m.detailsScroll += m.diffViewportHeight() / 2
		case "ctrl+u", "ctrl+b":
			m.detailsScroll -= m.diffViewportHeight() / 2
		default:
			m.showCommitDetails = false
			m.detailsScroll = 0
			m.pendingG = false
			return m, nil
		}
		// Clamp to the scrollable range so j past the end / k past the top stick.
		if max := m.detailsMaxScroll(); m.detailsScroll > max {
			m.detailsScroll = max
		}
		if m.detailsScroll < 0 {
			m.detailsScroll = 0
		}
		m.pendingG = false
		return m, nil
	}

	if m.searchActive {
		// Typing a diff-search query in the footer prompt. Enter commits it and
		// jumps to the first match; Esc cancels; everything else edits the buffer.
		switch msg.String() {
		case "esc":
			m.searchActive = false
			m.searchInput = ""
		case "enter":
			m.searchActive = false
			m.searchQuery = m.searchInput
			m.recomputeSearch()
			m.jumpToFirstHit()
		case "backspace":
			if r := []rune(m.searchInput); len(r) > 0 {
				m.searchInput = string(r[:len(r)-1])
			}
		default:
			if s := msg.String(); len([]rune(s)) == 1 {
				m.searchInput += s
			}
		}
		return m, nil
	}

	if m.fileFindActive {
		// The fuzzy file-jump picker is open. Enter jumps to the selected file;
		// Esc closes; arrows / ctrl+n/p move the selection; the rest edits the query.
		matches := m.fileFindMatches()
		switch msg.String() {
		case "esc":
			m.fileFindActive = false
		case "enter":
			if m.fileFindSel >= 0 && m.fileFindSel < len(matches) {
				m.jumpToFile(matches[m.fileFindSel].path)
			}
			m.fileFindActive = false
		case "down", "ctrl+n":
			if m.fileFindSel < len(matches)-1 {
				m.fileFindSel++
			}
		case "up", "ctrl+p":
			if m.fileFindSel > 0 {
				m.fileFindSel--
			}
		case "backspace":
			if r := []rune(m.fileFindInput); len(r) > 0 {
				m.fileFindInput = string(r[:len(r)-1])
			}
			m.fileFindSel = 0
		default:
			if s := msg.String(); len([]rune(s)) == 1 {
				m.fileFindInput += s
				m.fileFindSel = 0
			}
		}
		return m, nil
	}

	if m.showThemePicker {
		// The theme picker is open. j/k preview a theme live; T/tab previews the
		// light↔dark flip; i cycles the icon tier and m toggles motion (both apply
		// immediately); enter keeps the preview; esc reverts to the open snapshot.
		// All exits persist the resulting preferences.
		switch msg.String() {
		case "esc":
			m.setTheme(m.snapTheme, m.snapDark)
			m.showThemePicker = false
			m.persistPrefs()
		case "enter":
			m.showThemePicker = false
			m.persistPrefs()
		case "j", "down", "ctrl+n":
			if m.themeSel < len(themes)-1 {
				m.themeSel++
			}
			m.setTheme(m.themeSel, m.dark)
		case "k", "up", "ctrl+p":
			if m.themeSel > 0 {
				m.themeSel--
			}
			m.setTheme(m.themeSel, m.dark)
		case "T", "tab":
			m.setTheme(m.themeSel, !m.dark)
		case "i":
			m.iconSet = (m.iconSet + 1) % iconSet(len(iconSetNames))
		case "m":
			m.reduceMotion = !m.reduceMotion
			if !m.reduceMotion && !m.ticking {
				m.ticking = true
				return m, tickCmd(tickSlow)
			}
		}
		return m, nil
	}

	key := msg.String()

	// `gg` chord: a pending `g` followed by another `g` jumps to the top of
	// the focused pane. Any other key cancels the chord and is handled below.
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.gotoTop()
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Esc only steps back one level toward the history view (the default): a
		// per-commit (or working-tree) drill-in → the history list; the overview →
		// wherever it was opened from (the branch diff, the history list, or the
		// per-commit tree). On the history view it closes the diff pane (back to the
		// full-screen commit list), or does nothing when that's already the state —
		// use q / ctrl+c to quit.
		switch m.mode {
		case viewCommit:
			m.exitCommit()
		case viewBranch:
			m.enterLog()
		case viewAuthorDetail:
			m.exitAuthorDetail()
		case viewOverview:
			m.exitOverview()
		case viewLog:
			if m.logDiffOpen {
				m.closeLogDiff()
			}
		}
		return m, nil

	case "L":
		return m, m.goHistory()

	case "D":
		return m, m.goBranchDiff()

	case "?":
		m.showHelp = true
		return m, nil

	case "d":
		// Inspect the in-scope commit's details. Only meaningful where a real
		// commit is in scope: the highlighted commit in the history list (not the
		// working-tree row) or the commit being drilled into.
		if m.detailsCommit() != nil {
			m.showCommitDetails = true
			m.detailsScroll = 0
		}
		return m, nil

	case "r":
		repaint := m.refresh()
		m.syncFingerprint = git.Fingerprint(m.repo, m.baseName)
		if m.mode == viewOverview || m.mode == viewAuthorDetail {
			return m, tea.Batch(repaint, m.ensureHistory())
		}
		return m, repaint

	case "S":
		return m, m.toggleStats()

	case "s":
		// Toggle unified ↔ side-by-side. Row counts differ between modes, so
		// reset to the top to keep the scroll position sensible. Expansions are
		// about which lines are revealed, not layout, so they're preserved.
		// Inert in the commit-history view, whose right pane is a fixed per-commit
		// preview rather than an interactive diff.
		if m.mode == viewLog {
			return m, nil
		}
		m.splitView = !m.splitView
		m.diffOffset = 0
		m.diffCursor = 0
		m.rebuildView()
		m.clampDiffHOffset()
		return m, nil

	case "t":
		// Open the theme picker, snapshotting the current theme/variant so esc can
		// revert a live preview.
		m.showThemePicker = true
		m.themeSel = m.themeIdx
		m.snapTheme = m.themeIdx
		m.snapDark = m.dark
		return m, nil

	case "ctrl+k":
		// Open global search: one query across commit messages, changed-file paths,
		// and the code in the branch diff, grouped by category.
		m.showGlobalSearch = true
		m.gsInput = ""
		m.gsSel = 0
		return m, nil

	case ":":
		// Open the command palette: a fuzzy-searchable list of every action.
		m.showPalette = true
		m.paletteInput = ""
		m.paletteSel = 0
		return m, nil

	// --- sidebar collapse / expand (`[` narrows toward hidden, `]` widens) ---
	case "]":
		// Widen the sidebar: hidden → normal → wide. A wide sidebar surfaces extra
		// commit metadata (author, date, tags) in the history list. Disabled in the
		// commit-history view, where the sidebar is pinned to ~half the pane.
		if m.mode != viewOverview && m.mode != viewLog && m.sidebar < sidebarWide {
			m.sidebar++
		}
		return m, nil
	case "[":
		// Narrow the sidebar: wide → normal → hidden. Collapsing fully hands the
		// whole width to the diff, so move focus there since the list is gone.
		if m.mode != viewOverview && m.mode != viewLog && m.sidebar > sidebarHidden {
			m.sidebar--
			if m.sidebar == sidebarHidden {
				m.focus = focusDiff
			}
		}
		return m, nil

	// --- diff search (`/` to enter a query, n/N to step through matches) ---
	case "/":
		// Only meaningful where a diff is showing; harmless (an empty prompt) on a
		// folder or empty tree. Not in the overview, which has no diff pane.
		if m.mode != viewOverview {
			m.searchActive = true
			m.searchInput = ""
		}
		return m, nil
	case "n":
		m.nextHit()
		return m, nil
	case "N":
		m.prevHit()
		return m, nil

	// --- fuzzy file jump (only in the file-tree modes) ---
	case "f":
		if m.mode == viewBranch || m.mode == viewCommit {
			m.fileFindActive = true
			m.fileFindInput = ""
			m.fileFindSel = 0
		}
		return m, nil

	// --- pane focus (Tab) + horizontal diff scroll (h/l) ---
	case "tab":
		m.toggleFocus()
		return m, nil
	case "h", "left":
		// In the history view the diff pane is closed by default; there's nothing to
		// scroll until Tab opens it.
		if m.mode == viewLog && !m.logDiffOpen {
			return m, nil
		}
		m.scrollDiffH(-hScrollStep)
		return m, nil
	case "l", "right":
		// In the history view the diff pane is closed by default; Tab opens it on the
		// right half (Esc closes it). `l` only scrolls once it's open.
		if m.mode == viewLog && !m.logDiffOpen {
			m.openLogDiff()
			return m, nil
		}
		m.scrollDiffH(hScrollStep)
		return m, nil
	case "enter", "o":
		// Stats dashboard: open the selected contributor's detail page (kicking the
		// lazy module computation). The detail page itself has nothing further to open.
		if m.mode == viewOverview {
			return m, m.enterAuthorDetail()
		}
		if m.mode == viewAuthorDetail {
			return m, nil
		}
		// History list: enter drills into the highlighted commit's file tree, or
		// the working-tree row into the full branch view. (Scroll its preview
		// instead with l/Tab to focus the diff pane.)
		if m.mode == viewLog {
			if m.onWorkingRow() {
				m.enterWorkingTree()
			} else {
				m.enterCommit()
			}
			return m, nil
		}
		// In the diff pane, on an expand affordance: reveal hidden context.
		if m.focus == focusDiff {
			if l := m.cursorLine(); l != nil && l.Kind == diff.Expand {
				m.expandUnderCursor(*l)
			}
			return m, nil
		}
		// On a file row: open its diff and move into the diff pane. (The tree
		// cursor only ever rests on files — folder rows are skipped by j/k — so
		// there's no folder-fold case to handle here.)
		m.focus = focusDiff
		return m, nil

	// --- motions within the focused pane ---
	case "j", "down":
		m.moveDown()
		return m, nil
	case "k", "up":
		m.moveUp()
		return m, nil
	case "g":
		m.pendingG = true // wait for the second `g`
		return m, nil
	case "G", "end":
		m.gotoBottom()
		return m, nil

	// --- paging: half/full page through the focused pane ---
	case "ctrl+d":
		m.pageFocused(m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+u":
		m.pageFocused(-m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+f", "pgdown", " ":
		m.pageFocused(m.diffViewportHeight())
		return m, nil
	case "ctrl+b", "pgup":
		m.pageFocused(-m.diffViewportHeight())
		return m, nil
	}
	return m, nil
}

// setTheme applies theme idx in the given light/dark variant, rebuilding the
// global style table. The highlight cache memoizes token colors from the active
// palette, so it's dropped here to re-lex under the new colors (scroll position
// is preserved — only the colors change).
func (m *model) setTheme(idx int, dark bool) {
	if idx < 0 || idx >= len(themes) {
		idx = 0
	}
	m.themeIdx = idx
	m.dark = dark
	ApplyTheme(themes[idx], dark)
	m.hlCache = map[string][]span{}
}

// persistPrefs writes the current theme/icon/motion preferences to the config
// file (best-effort — a write failure is silently ignored, the in-session choice
// still stands).
func (m model) persistPrefs() {
	dark := m.dark
	_ = saveConfig(userConfig{
		Theme:        themes[m.themeIdx].Name,
		Icons:        m.iconSet.String(),
		ReduceMotion: m.reduceMotion,
		Dark:         &dark,
	})
}

func (m *model) toggleFocus() {
	// History view: Tab toggles the diff pane on/off — open it (focusing it) when
	// closed, hand the whole width back to the commit list when open. Focus moves
	// between the two panes with h/l.
	if m.mode == viewLog {
		if m.logDiffOpen {
			m.closeLogDiff()
		} else {
			m.openLogDiff()
		}
		return
	}
	// A collapsed sidebar has nothing to focus, so the diff stays the only target.
	if m.sidebar == sidebarHidden {
		m.focus = focusDiff
		return
	}
	if m.focus == focusFiles {
		m.focus = focusDiff
	} else {
		m.focus = focusFiles
	}
}

// moveDown/moveUp dispatch j/k to the focused pane: in viewLog the left pane is
// the commit list, otherwise it's the file tree; the right pane is always the
// diff's line cursor.
func (m *model) moveDown() {
	switch {
	case m.mode == viewOverview:
		m.moveOverviewCursor(1)
	case m.focus != focusFiles:
		m.moveDiffCursor(1)
	case m.mode == viewLog:
		m.moveCommitCursor(1)
	default:
		m.moveCursor(1)
	}
}

func (m *model) moveUp() {
	switch {
	case m.mode == viewOverview:
		m.moveOverviewCursor(-1)
	case m.focus != focusFiles:
		m.moveDiffCursor(-1)
	case m.mode == viewLog:
		m.moveCommitCursor(-1)
	default:
		m.moveCursor(-1)
	}
}

func (m *model) gotoTop() {
	switch {
	case m.mode == viewOverview:
		m.overviewCursor = 0
		m.ensureAuthorVisible()
	case m.focus != focusFiles:
		m.diffCursor = 0
		m.ensureCursorVisible()
	case m.mode == viewLog:
		m.commitCursor = 0
		if m.logDiffOpen {
			m.loadCommitDiff()
		}
	default:
		m.cursor = 0
		m.snapCursorToFile()
		m.loadDiff()
	}
}

func (m *model) gotoBottom() {
	switch {
	case m.mode == viewOverview:
		m.overviewCursor = max(0, len(m.historyStats.Authors)-1)
		m.ensureAuthorVisible()
	case m.focus != focusFiles:
		m.diffCursor = m.totalDiffRows() - 1
		m.ensureCursorVisible()
	case m.mode == viewLog:
		m.commitCursor = max(0, m.logRowCount()-1)
		if m.logDiffOpen {
			m.loadCommitDiff()
		}
	default:
		m.cursor = max(0, len(m.rows)-1)
		m.snapCursorToFile()
		m.loadDiff()
	}
}

// pageFocused steps the focused pane by delta rows for the half/full-page keys.
// In the commit-history list it pages the commit selection (like j/k there);
// everywhere else it scrolls the diff pane.
func (m *model) pageFocused(delta int) {
	if m.mode == viewOverview {
		m.moveOverviewCursor(delta)
		return
	}
	if m.mode == viewLog && m.focus == focusFiles {
		m.moveCommitCursor(delta)
		return
	}
	m.moveDiffCursor(delta)
}

// moveDiffCursor moves the diff-pane line cursor and scrolls to keep it visible.
func (m *model) moveDiffCursor(delta int) {
	m.diffCursor += delta
	m.clampDiffCursor()
	m.ensureCursorVisible()
}

// expandUnderCursor reveals the next window of hidden context for the gap under
// the cursor: downward (or "all") grows the revealed block from the top, upward
// grows it from the bottom. The view is rebuilt and the cursor held in place.
func (m *model) expandUnderCursor(l diff.Line) {
	if l.GapID < 0 || l.GapID >= len(m.gaps) {
		return
	}
	rev := m.revealed[l.GapID]
	if l.Dir == diff.ExpandUp {
		rev[1] += expandWindow
	} else {
		rev[0] += expandWindow
	}
	m.revealed[l.GapID] = rev
	m.rebuildView()
}

// moveCursor steps the tree cursor by delta, skipping folder rows so j/k always
// land on a file with a real diff. The step direction (sign of delta) is also the
// scan direction: a `j` onto a folder keeps moving down to the next file, a `k`
// keeps moving up. If no file lies that way, the cursor stays put.
func (m *model) moveCursor(delta int) {
	if len(m.files) == 0 || delta == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	i := m.cursor
	for n := abs(delta); n > 0; {
		i += step
		if i < 0 || i >= len(m.rows) {
			break
		}
		if m.rows[i].file != nil {
			m.cursor = i
			n--
		}
	}
	m.loadDiff()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// refresh recomputes the base and reloads the changed-file list from disk so the
// view reflects edits made since launch. It is driven both by the manual `r` key
// and by the background sync poll, so it preserves the reader's position: the
// same file stays selected by path (the same commit by SHA in history), and diff
// scroll/cursor/expansion are kept when the visible content didn't actually
// change — only genuinely new content resets the view.
func (m *model) refresh() (repaint tea.Cmd) {
	m.base = git.BaseRef(m.repo, m.baseName)
	// Keep the branch label current: a checkout while diffcat is open moves HEAD to
	// a different branch, and the header reads from m.branch.
	m.branch = git.CurrentBranch(m.repo)
	// Drop the global-search corpora so the next search rebuilds them against the
	// refreshed working tree rather than a stale snapshot.
	m.gsFiles = nil
	m.gsCode = nil
	// The whole-history Summary depends only on committed history, so invalidate
	// its cached stats only when HEAD actually moved — a working-tree edit (the
	// common refresh trigger) doesn't change history, and recomputing the full
	// `git log` on every save would be wasteful on a deep repo.
	if head := git.HeadSHA(m.repo); head != m.historyHead {
		m.historyHead = head
		m.historyComputed = false
		m.historyComputing = false
		// The per-author SHA sets (and so their module rankings) move with history;
		// drop the lazy cache so a reopened contributor recomputes against the new HEAD.
		m.authorModules = map[string][]git.ModuleCount{}
		m.authorModulesComputing = map[string]bool{}
		// HEAD moving (a commit, checkout, rebase, or reset) restructures the commit
		// list and file tree — rows shift, the base divider appears or disappears.
		// That can trip bubbletea's scroll-diff optimization (e.g. the base tip shows
		// at a different row across the two frames), which then leaves stale rows on
		// screen — most visibly an apparently-frozen commit history after switching
		// branches. Force a clean full repaint so the new state always paints whole.
		repaint = tea.ClearScreen
	}

	if m.mode == viewOverview || m.mode == viewAuthorDetail {
		// The full-screen Stats dashboard (and a contributor's detail page) cover
		// whatever they were opened from — and that origin may have stashed its own
		// tree (a per-commit drill-in repurposes m.files). Leave all that origin state
		// untouched so backing out restores it intact; it re-syncs on the next poll
		// once the dashboard closes. Only the Stats themselves were refreshed (above).
		return
	}

	if m.mode == viewCommit {
		if m.scopeWorking {
			// The working-tree drill-in is mutable (edits/staging change its file
			// set), so rebuild it from `git diff HEAD`, holding the selected file
			// and scroll where possible. The stashed branch tree re-syncs on exit.
			prevPath := ""
			if f := m.selectedFile(); f != nil {
				prevPath = f.Path
			}
			if files, err := git.ChangedFiles(m.repo, "HEAD"); err == nil {
				m.files = files
				m.rebuildTree()
				m.reselectPath(prevPath)
			}
			m.preserveDiffView(m.loadDiff)
			return
		}
		// A real commit's file set is immutable, so it (and the stashed branch
		// tree) are left untouched; just reload the visible file's diff, holding
		// scroll. The branch tree re-syncs once you exit back to it.
		m.preserveDiffView(m.loadDiff)
		return
	}

	prevPath := ""
	if f := m.selectedFile(); f != nil {
		prevPath = f.Path
	}
	if files, err := git.ChangedFiles(m.repo, m.base); err == nil {
		m.files = files
		// The header summary rides the counts the change list already carries — no
		// extra `git diff --shortstat`. Only the branch/log views (this fall-through
		// path) display it; viewCommit/viewOverview returned earlier and leave it.
		m.shortstat = git.ShortstatOf(files)
		m.rebuildTree()
		m.reselectPath(prevPath)
		m.flagChangedFiles()
	}

	if m.mode == viewLog {
		m.reloadCommitList()
		m.preserveDiffView(m.loadCommitDiff)
		return
	}
	m.preserveDiffView(m.loadDiff)
	return
}

// reselectPath moves the tree cursor back onto the row for path after a rebuild,
// so a sync keeps the same file selected even when the change list reordered or
// grew. Falls back to the (already clamped) cursor when the path is gone — e.g.
// its changes were reverted.
func (m *model) reselectPath(path string) {
	if path == "" {
		return
	}
	for i, r := range m.rows {
		if r.file != nil && r.file.Path == path {
			m.cursor = i
			return
		}
	}
}

// reselectCommit moves the history cursor back onto the commit with sha after the
// list reloads, so a sync that added newer commits on top doesn't shift the
// reader onto a different commit. Falls back to the clamped cursor if it's gone.
func (m *model) reselectCommit(sha string) {
	if sha != "" {
		for i, c := range m.commits {
			if c.SHA == sha {
				m.commitCursor = i
				if m.logWorking {
					m.commitCursor++ // account for the leading working-tree row
				}
				return
			}
		}
	}
	m.clampCommitCursor()
}

// preserveDiffView runs reload (loadDiff or loadCommitDiff) and, when the diff it
// produces is byte-for-byte identical to what was on screen, restores the prior
// scroll, cursor, and expansion state — so a background sync that didn't touch
// the visible file leaves the reader exactly where they were. A changed diff is
// left reset to the top by reload, since the old position no longer maps onto it.
func (m *model) preserveDiffView(reload func()) {
	prevDiff := m.diff
	prevOffset, prevCursor := m.diffOffset, m.diffCursor
	prevRevealed := m.revealed
	reload()
	if diffLinesEqual(prevDiff, m.diff) {
		m.revealed = prevRevealed
		m.rebuildView()
		m.diffOffset = prevOffset
		m.diffCursor = prevCursor
		m.clampDiffOffset()
		m.clampDiffCursor()
	}
}

// diffLinesEqual reports whether two pristine diffs are identical line-for-line.
// It compares the scalar fields directly; the Emph ranges are a deterministic
// function of the lines' text and kind, so equal scalar fields imply equal Emph.
func diffLinesEqual(a, b []diff.Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.Text != y.Text || x.OldNum != y.OldNum ||
			x.NewNum != y.NewNum || x.Path != y.Path || x.Dir != y.Dir ||
			x.GapID != y.GapID || x.Hidden != y.Hidden {
			return false
		}
	}
	return true
}

// goHistory returns to the commit-history view (the default) from anywhere. From
// a per-commit tree it steps back to the list rather than reloading from scratch;
// a contributor's detail page normalizes through the ranking it sits above; the
// overview unwinds through its origin so a commit overview's stashed branch tree
// is restored. Shared by the `L` key and the command palette.
func (m *model) goHistory() tea.Cmd {
	if m.mode == viewAuthorDetail {
		m.exitAuthorDetail()
	}
	switch m.mode {
	case viewLog:
		// already in history
	case viewCommit:
		m.exitCommit()
	case viewOverview:
		m.exitOverview()
		if m.mode == viewCommit {
			m.exitCommit()
		} else if m.mode != viewLog {
			m.enterLog()
		}
	default:
		m.enterLog()
	}
	return nil
}

// goBranchDiff opens the aggregated branch-vs-base diff (the file tree + diff).
// From a per-commit/working-tree drill-in it restores the branch tree on the way;
// from the overview it drops back to the branch diff it (or its origin)
// summarizes. On the base branch the diff is degenerate (merge base = HEAD), so
// it's inert. Shared by the `D` key and the command palette.
func (m *model) goBranchDiff() tea.Cmd {
	if m.onBaseBranch() {
		return nil
	}
	if m.mode == viewAuthorDetail {
		m.exitAuthorDetail()
	}
	switch m.mode {
	case viewBranch:
		// already showing the branch diff
	case viewCommit:
		m.exitCommit()
		m.exitLog()
	case viewOverview:
		m.exitOverview()
		switch m.mode {
		case viewCommit:
			m.exitCommit()
			m.exitLog()
		case viewLog:
			m.exitLog()
		}
	default:
		m.exitLog()
	}
	return nil
}

// toggleStats toggles the whole-repo Stats dashboard, which belongs to the
// commit-history view. From a contributor's detail page it drops to the ranking
// first so one press toggles the whole dashboard off. Shared by `S` and the
// command palette.
func (m *model) toggleStats() tea.Cmd {
	if m.mode == viewAuthorDetail {
		m.exitAuthorDetail()
	}
	switch m.mode {
	case viewOverview:
		m.exitOverview()
	case viewLog:
		return m.enterOverview()
	}
	return nil
}

// clampCommitCursor keeps the history cursor within the (possibly reloaded)
// commit list.
func (m *model) clampCommitCursor() {
	if m.commitCursor >= m.logRowCount() {
		m.commitCursor = max(0, m.logRowCount()-1)
	}
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
}

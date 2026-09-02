package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/inre/tmux2/internal/layout"
	"github.com/inre/tmux2/internal/protocol"
	"github.com/inre/tmux2/internal/ptyx"
	"github.com/inre/tmux2/internal/render"
	"github.com/inre/tmux2/internal/vterm"
)

// sessionOpts carries the configuration a session needs at creation time.
type sessionOpts struct {
	historyLimit       int
	defaultShell       string // empty => platform default
	statusFG, statusBG int

	// newPane builds a pane. Nil means the production factory (startPane);
	// tests inject a fake so session/window logic runs without a real shell.
	newPane paneFactory
}

// session is a named workspace holding an ordered list of windows, one of
// which is current. All state is guarded by mu; window and pane methods are
// only ever called with it held.
type session struct {
	mu sync.Mutex

	name    string
	windows []*window
	cur     int
	created time.Time
	opts    sessionOpts

	nextPaneID   layout.PaneID
	nextWindowID int

	cols, rows int // viewport of the attached client(s); rows includes status

	pasteBuf   string      // last copy-mode yank; target of the paste command
	drag       dragState   // in-progress mouse border drag
	statusHits []statusHit // clickable status-bar regions, rebuilt each frame

	// needsRepaint forces the next frame even if no pane produced output
	// (a command changed focus, layout, a window name, ...). frame() clears it.
	needsRepaint bool

	// Render scratch, touched only by frame() on the single broadcast
	// goroutine: two frames ping-ponged so a steady repaint allocates nothing,
	// plus reused pane-view and snapshot buffers.
	frameBuf    [2]*render.Frame
	frameIdx    int
	viewScratch []render.PaneView
	snapScratch []vterm.Snapshot

	onEmpty  func(name string) // called once when the last window is gone
	paneExit chan *pane
	dead     chan struct{}
	deadOnce sync.Once
}

func newSession(name string, cols, rows int, opts sessionOpts, onEmpty func(string)) (*session, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if opts.newPane == nil {
		opts.newPane = startPane
	}
	s := &session{
		name:     name,
		created:  time.Now(),
		opts:     opts,
		cols:     cols,
		rows:     rows,
		onEmpty:  onEmpty,
		paneExit: make(chan *pane, 32),
		dead:     make(chan struct{}),
	}
	if _, err := s.spawnWindow(cols, s.contentRows()); err != nil {
		return nil, err
	}
	go s.reap()
	return s, nil
}

// spawnWindow creates a window, starts its pane's output pump, appends it to
// the session, and makes it current. It is the single path for adding a
// window: session creation, the new-window command, and the status-bar [+]
// button all go through here. Caller holds mu (except during newSession, where
// the session is not yet shared).
func (s *session) spawnWindow(cols, rows int) (*window, error) {
	w, err := newWindow(s.nextWin(), s.defaultWindowName(), s.nextPane(), cols, rows,
		s.opts.defaultShell, s.opts.historyLimit, s.opts.newPane)
	if err != nil {
		return nil, err
	}
	for _, p := range w.panes {
		go p.pump(s.reportExit)
	}
	s.windows = append(s.windows, w)
	s.cur = len(s.windows) - 1
	return w, nil
}

func (s *session) defaultWindowName() string {
	sh := s.opts.defaultShell
	if sh == "" {
		sh = ptyx.DefaultShell()
	}
	return strings.TrimSuffix(filepath.Base(sh), ".exe")
}

func (s *session) nextPane() layout.PaneID {
	s.nextPaneID++
	return s.nextPaneID
}

func (s *session) nextWin() int {
	id := s.nextWindowID
	s.nextWindowID++
	return id
}

func (s *session) contentRows() int {
	if s.rows <= 1 {
		return s.rows
	}
	return s.rows - 1
}

func (s *session) reportExit(p *pane) {
	select {
	case s.paneExit <- p:
	case <-s.dead:
	}
}

// reap folds finished panes out of the layout and, when a window loses its
// last pane, drops the window. When the session loses its last window it
// notifies the server and stops.
func (s *session) reap() {
	for {
		select {
		case <-s.dead:
			return
		case p := <-s.paneExit:
			s.mu.Lock()
			if w := p.win; w != nil {
				w.removePane(p.id)
				if len(w.panes) == 0 {
					s.removeWindow(w)
				}
			}
			empty := len(s.windows) == 0
			s.mu.Unlock()
			if empty {
				if s.onEmpty != nil {
					s.onEmpty(s.name)
				}
				return
			}
		}
	}
}

// removeWindow drops w from the list by identity and clamps cur. Caller holds mu.
func (s *session) removeWindow(w *window) {
	for i, x := range s.windows {
		if x == w {
			s.removeWindowAt(i)
			return
		}
	}
}

func (s *session) removeWindowAt(i int) {
	s.windows = append(s.windows[:i:i], s.windows[i+1:]...)
	if s.cur >= len(s.windows) {
		s.cur = len(s.windows) - 1
	}
	if s.cur < 0 {
		s.cur = 0
	}
}

func (s *session) resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols <= 0 || rows <= 0 || (cols == s.cols && rows == s.rows) {
		return
	}
	s.cols, s.rows = cols, rows
	content := s.contentRows()
	for _, w := range s.windows {
		w.applyLayout(cols, content)
	}
}

// input routes raw bytes: to the focused pane's copy-mode scroller if it is in
// copy-mode, otherwise straight to its pty. It returns true when the client
// should be sent a full repaint (copy-mode navigation moves the whole view).
func (s *session) input(data []byte) (repaint bool) {
	s.mu.Lock()
	var p *pane
	if w := s.current(); w != nil {
		p = w.panes[w.active]
	}
	if p != nil && p.copy != nil {
		if text, _ := p.copyKey(data); text != "" {
			s.pasteBuf = text
		}
		s.mu.Unlock()
		return true
	}
	s.mu.Unlock()
	if p != nil {
		_, _ = p.pt.Write(data)
	}
	return false
}

// current returns the current window, or nil if the session has none. Caller
// holds mu.
func (s *session) current() *window {
	if len(s.windows) == 0 {
		return nil
	}
	if s.cur < 0 || s.cur >= len(s.windows) {
		s.cur = 0
	}
	return s.windows[s.cur]
}

// withWindow runs fn with the session locked and the current window plus its
// content-area size. It is the common preamble for command handlers that act on
// the current window; a session with no window is a silent no-op.
func (s *session) withWindow(fn func(w *window, cols, contentRows int) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.current()
	if w == nil {
		return nil
	}
	return fn(w, s.cols, s.contentRows())
}

// selectWindow makes window idx current, if idx is in range.
func (s *session) selectWindow(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx >= 0 && idx < len(s.windows) {
		s.cur = idx
	}
}

// stepWindow moves the current-window pointer by delta, wrapping around.
func (s *session) stepWindow(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.windows); n > 0 {
		s.cur = (s.cur + delta%n + n) % n
	}
}

// frame renders the current window plus the status bar into one of the
// session's two reusable frame buffers. It clears needsRepaint: after this call
// the session is "clean" until something changes again. Only ever called from
// the broadcast goroutine.
func (s *session) frame() *render.Frame {
	s.mu.Lock()
	s.needsRepaint = false
	cols, rows := s.cols, s.rows
	content := s.contentRows()
	w := s.current()
	var views []render.PaneView
	if w != nil {
		views, s.snapScratch = w.views(cols, content, s.viewScratch[:0], s.snapScratch)
		s.viewScratch = views
	}
	status := s.buildStatus(cols)
	style := render.StatusStyle{FG: s.opts.statusFG, BG: s.opts.statusBG}
	s.mu.Unlock()

	f := render.ComposeStyledInto(s.frameBuf[s.frameIdx], cols, rows, views, status, style)
	s.frameBuf[s.frameIdx] = f
	s.frameIdx ^= 1
	return f
}

// dirty reports whether the next frame would differ from the last one frame()
// produced: a pending needsRepaint, or a pane that consumed new output.
func (s *session) dirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.needsRepaint {
		return true
	}
	for _, w := range s.windows {
		for _, p := range w.panes {
			if p.vt.Dirty() {
				return true
			}
		}
	}
	return false
}

// statusHit is a clickable region of the status bar: [X0,X1) maps to an action.
type statusHit struct {
	x0, x1 int
	action string // "select-window" (win in N) or "new-window"
	n      int
}

// statusHint is the right-aligned reminder of the two commands a new user most
// often needs: how to close the focused pane and how to leave the session.
const statusHint = "^b x close pane · ^b d exit "

// buildStatus lays out "[name] 0:win 1:win* … [+]" on the left, then a hint and
// a clock on the right, padded to the full width. The active window, the
// session name and the [+] button are bold so the bar reads at a glance. It
// also records clickable regions in s.statusHits; the left-hand layout (and so
// every hit column) is byte-for-byte what the string version produced. Caller
// holds mu.
func (s *session) buildStatus(cols int) []render.StatusSegment {
	s.statusHits = s.statusHits[:0]

	var segs []render.StatusSegment
	col := 0 // running rune column, for click regions and padding
	add := func(text string, attr uint16) {
		if text == "" {
			return
		}
		segs = append(segs, render.StatusSegment{
			Text: text, FG: render.InheritColour, BG: render.InheritColour, Attr: attr,
		})
		col += utf8.RuneCountInString(text)
	}

	add(fmt.Sprintf(" [%s] ", s.name), vterm.AttrBold)

	for i, w := range s.windows {
		mark := " "
		attr := uint16(0)
		if i == s.cur {
			mark, attr = "*", vterm.AttrBold
		}
		entry := fmt.Sprintf("%d:%s%s ", i, w.name, mark)
		x0 := col
		add(entry, attr)
		s.statusHits = append(s.statusHits, statusHit{
			x0: x0, x1: x0 + utf8.RuneCountInString(entry) - 1, // exclude the trailing space
			action: "select-window", n: i,
		})
	}

	plusX0 := col
	add("[+] ", vterm.AttrBold)
	s.statusHits = append(s.statusHits, statusHit{x0: plusX0, x1: plusX0 + 2, action: "new-window"})

	if w := s.current(); w != nil {
		if p := w.panes[w.active]; p != nil && p.copy != nil {
			add("-- COPY -- ", vterm.AttrBold)
		}
	}

	clock := time.Now().Format("15:04") + " "
	// Prefer "<hint> <clock>"; if that will not fit, drop the hint; if even the
	// clock will not fit, leave the row to be clipped at the frame edge.
	right := statusHint + clock
	if col+1+utf8.RuneCountInString(right) > cols {
		right = clock
	}
	if pad := cols - col - utf8.RuneCountInString(right); pad >= 1 {
		add(strings.Repeat(" ", pad), 0)
		if right != clock {
			add(statusHint, 0)
		}
		add(clock, vterm.AttrBold)
	}
	return segs
}

// statusClick runs the action for a click at column x on the status row, if any
// clickable region covers it. Caller holds mu. It returns true if it acted.
func (s *session) statusClick(x int) bool {
	for _, h := range s.statusHits {
		if x < h.x0 || x > h.x1 {
			continue
		}
		switch h.action {
		case "select-window":
			if h.n >= 0 && h.n < len(s.windows) {
				s.cur = h.n
			}
		case "new-window":
			if _, err := s.spawnWindow(s.cols, s.contentRows()); err != nil {
				return false
			}
		}
		return true
	}
	return false
}

func (s *session) info(attached bool) protocol.SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	panes := 0
	for _, w := range s.windows {
		panes += len(w.panes)
	}
	return protocol.SessionInfo{
		Name:     s.name,
		Windows:  len(s.windows),
		Panes:    panes,
		Attached: attached,
	}
}

func (s *session) shutdown() {
	s.deadOnce.Do(func() { close(s.dead) })
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.windows {
		w.closeAll()
	}
	s.windows = nil
}

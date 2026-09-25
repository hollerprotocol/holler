// Package tui is `holler watch`: a live dashboard of the agents on a holler
// network, built with Charm's Bubble Tea, Lip Gloss, Bubbles, Glamour and
// Harmonica. It reads the local daemon over the control socket; agents on
// other hosts appear through the presence gossip extension.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/wire"
)

// Options configure the dashboard.
type Options struct {
	// Log receives diagnostics (data errors, stream restarts). Nil
	// discards them; the terminal belongs to the dashboard.
	Log *log.Logger
}

type tab int

const (
	tabDashboard tab = iota
	tabNetwork
	tabActivity
)

var tabNames = []string{"Dashboard", "Network", "Activity"}

type pane int

const (
	paneAgents pane = iota
	paneThreads
	paneFeed
)

const (
	pollEvery   = 1500 * time.Millisecond
	feedLimit   = 2000
	freshFor    = 2 * time.Second // new feed lines glow this long
	historySize = 150
)

type model struct {
	ctx  context.Context
	src  Source
	log  *log.Logger
	keys keyMap

	w, h int
	dark bool
	th   theme

	help      help.Model
	spin      spinner.Model
	filter    textinput.Model
	filtering bool
	prog      progress.Model
	md        *markdown // shared by value copies of the model: View caches into it

	snap      *Snapshot
	snapErr   error
	fetching  bool
	lastFetch time.Time
	agents    []agentRow
	names     map[string]string
	self      string

	tab                 tab
	focus               pane
	agentSel, threadSel int
	agentTop, threadTop int
	agentBar, threadBar spring
	actOff              int // activity table scroll

	feed          []feedItem
	feedSeqs      map[int64]bool
	feedOff       int // lines scrolled up from the newest
	paused        bool
	events        chan api.Event
	streaming     bool
	historyLoaded bool

	detail  *detail
	preview *preview

	pulse     spring
	animating bool
	now       time.Time
	flash     string
	flashTill time.Time
}

// detail is an open conversation.
type detail struct {
	peer, th string
	events   []api.Event
	vp       viewport.Model
	loaded   bool
	err      error
}

func newModel(ctx context.Context, src Source, opts Options) model {
	m := model{
		ctx:       ctx,
		src:       src,
		log:       opts.Log,
		keys:      newKeyMap(),
		dark:      true,
		help:      help.New(),
		spin:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		filter:    textinput.New(),
		prog:      progress.New(progress.WithWidth(18), progress.WithDefaultBlend(), progress.WithoutPercentage()),
		md:        &markdown{},
		feedSeqs:  map[int64]bool{},
		events:    make(chan api.Event, 256),
		agentBar:  newSpring(9, 0.85),
		threadBar: newSpring(9, 0.85),
		pulse:     newSpring(4, 1),
		now:       time.Now(),
		names:     map[string]string{},
	}
	m.filter.Prompt = "/ "
	m.filter.Placeholder = "filter agents, threads and activity"
	m.applyTheme(true)
	return m
}

func (m *model) applyTheme(dark bool) {
	m.dark = dark
	m.th = newTheme(dark)
	m.help.Styles = help.DefaultStyles(dark)
	m.filter.SetStyles(textinput.DefaultStyles(dark))
	m.spin.Style = lipgloss.NewStyle().Foreground(m.th.warn)
}

func (m *model) logf(format string, args ...any) {
	if m.log != nil {
		m.log.Debugf(format, args...)
	}
}

// Run shows the dashboard until the user quits.
func Run(ctx context.Context, src Source, opts Options) error {
	p := tea.NewProgram(newModel(ctx, src, opts), tea.WithContext(ctx))
	_, err := p.Run()
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}

type (
	snapshotMsg struct {
		snap *Snapshot
		err  error
	}
	historyMsg struct {
		events []api.Event
		err    error
	}
	eventMsg  struct{ ev api.Event }
	threadMsg struct {
		peer, th string
		events   []api.Event
		err      error
	}
	pollMsg  struct{}
	clockMsg time.Time
	frameMsg struct{}
)

func (m model) fetchSnapshot() tea.Cmd {
	ctx, src := m.ctx, m.src
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		s, err := src.Snapshot(c)
		return snapshotMsg{s, err}
	}
}

func (m model) fetchHistory() tea.Cmd {
	ctx, src := m.ctx, m.src
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		evs, err := src.History(c, historySize)
		return historyMsg{evs, err}
	}
}

func (m model) fetchThread(peer, th string) tea.Cmd {
	ctx, src := m.ctx, m.src
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		evs, err := src.Thread(c, peer, th)
		return threadMsg{peer, th, evs, err}
	}
}

func waitEvent(ch <-chan api.Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg{<-ch}
	}
}

func poll() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

func clock() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

func frame() tea.Cmd {
	return tea.Tick(time.Second/fps, func(time.Time) tea.Msg { return frameMsg{} })
}

// Init implements tea.Model. History is fetched after the first snapshot,
// once we know which key is ours.
func (m model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.fetchSnapshot(), m.spin.Tick, clock(), poll())
}

// animate starts the frame loop if it is not already running.
func (m *model) animate() tea.Cmd {
	if m.animating {
		return nil
	}
	m.animating = true
	return frame()
}

func (m *model) setFlash(s string) {
	m.flash, m.flashTill = s, time.Now().Add(3*time.Second)
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.filter.SetWidth(max(10, msg.Width-4))
		m.resizeDetail()
		return m, nil

	case tea.BackgroundColorMsg:
		m.applyTheme(msg.IsDark())
		m.renderDetail()
		return m, nil

	case snapshotMsg:
		m.fetching = false
		m.lastFetch = time.Now()
		if msg.err != nil {
			if m.snapErr == nil {
				m.logf("snapshot: %v", msg.err)
			}
			m.snapErr = msg.err
			return m, nil
		}
		m.snapErr = nil
		prev := m.agents
		m.snap = msg.snap
		m.self = msg.snap.Status.Key
		m.agents, m.names = buildAgents(msg.snap)
		if prev != nil {
			for _, it := range diffPresence(prev, m.agents, m.self, time.Now()) {
				m.addFeed(it)
			}
		}
		m.clampSelection()
		cmds := []tea.Cmd{m.syncPreview()}
		if !m.historyLoaded {
			cmds = append(cmds, m.fetchHistory())
		}
		return m, tea.Batch(cmds...)

	case historyMsg:
		if msg.err != nil || m.historyLoaded {
			return m, nil
		}
		m.historyLoaded = true
		var after int64
		for _, ev := range msg.events {
			if ev.Seq > after {
				after = ev.Seq
			}
			it := itemFromEvent(ev, m.selfKeyFor(ev))
			m.addFeed(it)
		}
		if !m.streaming {
			m.streaming = true
			go stream(m.ctx, m.src, after, m.events)
		}
		return m, waitEvent(m.events)

	case eventMsg:
		ev := msg.ev
		it := itemFromEvent(ev, m.selfKeyFor(ev))
		it.Born = time.Now()
		m.addFeed(it)
		m.pulse.pos, m.pulse.vel, m.pulse.target = 1, 0, 0
		cmds := []tea.Cmd{waitEvent(m.events), m.animate()}
		if d := m.detail; d != nil && ev.Th == d.th && ev.Peer == d.peer {
			d.events = append(d.events, ev)
			m.renderDetail()
		}
		if p := m.preview; p != nil && p.loaded && ev.Th == p.th && ev.Peer == p.peer {
			p.add(ev)
		}
		if !m.fetching && time.Since(m.lastFetch) > 300*time.Millisecond {
			m.fetching = true
			cmds = append(cmds, m.fetchSnapshot())
		}
		return m, tea.Batch(cmds...)

	case threadMsg:
		if d := m.detail; d != nil && d.th == msg.th && d.peer == msg.peer {
			first := !d.loaded
			d.events, d.err, d.loaded = msg.events, msg.err, true
			m.renderDetail()
			if first {
				d.vp.GotoBottom()
			}
		}
		if p := m.preview; p != nil && p.th == msg.th && p.peer == msg.peer && msg.err == nil {
			p.events, p.seqs, p.loaded = nil, nil, true
			for _, ev := range msg.events {
				p.add(ev)
			}
		}
		return m, nil

	case pollMsg:
		cmds := []tea.Cmd{poll()}
		if !m.fetching {
			m.fetching = true
			cmds = append(cmds, m.fetchSnapshot())
		}
		// Acks change messages already shown, and the event stream only
		// carries new ones, so an open conversation waiting on one reloads.
		now := time.Now()
		d, p := m.detail, m.preview
		if d != nil && d.loaded && ackPending(d.events, now) {
			cmds = append(cmds, m.fetchThread(d.peer, d.th))
		}
		if p != nil && p.loaded && ackPending(p.events, now) && (d == nil || d.th != p.th || d.peer != p.peer) {
			cmds = append(cmds, m.fetchThread(p.peer, p.th))
		}
		return m, tea.Batch(cmds...)

	case clockMsg:
		m.now = time.Time(msg)
		cmds := []tea.Cmd{clock()}
		if m.freshItems() {
			cmds = append(cmds, m.animate())
		}
		return m, tea.Batch(cmds...)

	case frameMsg:
		moving := m.agentBar.step()
		moving = m.threadBar.step() || moving
		moving = m.pulse.step() || moving
		if moving || m.freshItems() {
			return m, frame()
		}
		m.animating = false
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.MouseWheelMsg:
		return m.wheel(msg)

	case tea.MouseClickMsg:
		return m.click(msg)

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// ackPending reports whether a message sent in the last half minute is
// still waiting for the peer's ack. Older ones are left to a refresh: the
// peer is likely offline.
func ackPending(evs []api.Event, now time.Time) bool {
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.Dir == "out" && ev.Type == wire.TMsg && !ev.Acked && now.Sub(ev.At) < 30*time.Second {
			return true
		}
	}
	return false
}

// selfKeyFor is this host's key; events carry the remote peer.
func (m model) selfKeyFor(api.Event) string { return m.self }

func (m *model) addFeed(it feedItem) {
	if it.Seq != 0 {
		if m.feedSeqs[it.Seq] {
			return
		}
		m.feedSeqs[it.Seq] = true
	}
	m.feed = append(m.feed, it)
	if len(m.feed) > feedLimit {
		drop := m.feed[:len(m.feed)-feedLimit]
		for _, d := range drop {
			delete(m.feedSeqs, d.Seq)
		}
		m.feed = append([]feedItem(nil), m.feed[len(m.feed)-feedLimit:]...)
	}
	if m.paused && m.feedOff > 0 {
		m.feedOff++ // keep the view still while paused
	}
}

func (m model) freshItems() bool {
	for i := len(m.feed) - 1; i >= 0 && i >= len(m.feed)-20; i-- {
		if !m.feed[i].Born.IsZero() && time.Since(m.feed[i].Born) < freshFor {
			return true
		}
	}
	return false
}

// --- filtering and selection ---

func (m model) query() string {
	return strings.ToLower(strings.TrimSpace(m.filter.Value()))
}

func (m model) visibleAgents() []agentRow {
	q := m.query()
	if q == "" {
		return m.agents
	}
	var out []agentRow
	for _, a := range m.agents {
		hay := strings.ToLower(a.Name + " " + a.Key + " " + a.About + " " + a.Status)
		match := strings.Contains(hay, q)
		for _, t := range a.Threads {
			if strings.Contains(strings.ToLower(t.Subject+" "+t.PeerName+" "+t.Mine), q) {
				match = true
			}
		}
		if match {
			out = append(out, a)
		}
	}
	return out
}

func (m model) selectedAgent() *agentRow {
	as := m.visibleAgents()
	if len(as) == 0 {
		return nil
	}
	a := as[min(m.agentSel, len(as)-1)]
	return &a
}

func (m model) visibleThreads() []threadRow {
	a := m.selectedAgent()
	if a == nil {
		return nil
	}
	q := m.query()
	var out []threadRow
	for _, t := range a.Threads {
		if q != "" && !strings.Contains(strings.ToLower(a.Name+" "+t.Subject+" "+t.PeerName+" "+t.Mine+" "+t.Theirs+" "+t.Th), q) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (m model) visibleFeed() []feedItem {
	q := m.query()
	if q == "" {
		return m.feed
	}
	var out []feedItem
	for _, it := range m.feed {
		if strings.Contains(it.haystack(m.names), q) {
			out = append(out, it)
		}
	}
	return out
}

func (m *model) clampSelection() {
	if n := len(m.visibleAgents()); m.agentSel >= n {
		m.agentSel = max(0, n-1)
	}
	if n := len(m.visibleThreads()); m.threadSel >= n {
		m.threadSel = max(0, n-1)
	}
	m.agentBar.target = float64(m.agentSel)
	m.threadBar.target = float64(m.threadSel)
}

func (m *model) moveSel(delta int) tea.Cmd {
	switch m.focus {
	case paneAgents:
		n := len(m.visibleAgents())
		if n == 0 {
			return nil
		}
		m.agentSel = max(0, min(n-1, m.agentSel+delta))
		m.threadSel, m.threadTop = 0, 0
		m.threadBar.pos, m.threadBar.target = 0, 0
		m.agentBar.target = float64(m.agentSel)
	case paneThreads:
		n := len(m.visibleThreads())
		if n == 0 {
			return nil
		}
		m.threadSel = max(0, min(n-1, m.threadSel+delta))
		m.threadBar.target = float64(m.threadSel)
	case paneFeed:
		m.feedOff = max(0, m.feedOff-delta)
		return nil
	}
	return tea.Batch(m.animate(), m.syncPreview())
}

// --- input ---

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	if m.filtering {
		switch msg.String() {
		case "enter":
			m.filtering = false
			m.filter.Blur()
			m.clampSelection()
			return m, nil
		case "esc":
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.clampSelection()
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.agentSel, m.threadSel, m.agentTop, m.threadTop = 0, 0, 0, 0
		m.clampSelection()
		return m, tea.Batch(cmd, m.syncPreview())
	}
	if key.Matches(msg, k.Quit) {
		return m, tea.Quit
	}
	if m.detail != nil {
		switch {
		case key.Matches(msg, k.Back):
			m.detail = nil
			return m, nil
		case msg.String() == "r":
			return m, m.fetchThread(m.detail.peer, m.detail.th)
		}
		var cmd tea.Cmd
		m.detail.vp, cmd = m.detail.vp.Update(msg)
		return m, cmd
	}
	switch {
	case key.Matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, k.Filter):
		m.filtering = true
		return m, m.filter.Focus()
	case key.Matches(msg, k.Back):
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.clampSelection()
		} else if m.tab != tabDashboard {
			m.tab = tabDashboard
		}
	case key.Matches(msg, k.Dashboard):
		m.tab = tabDashboard
	case key.Matches(msg, k.Network):
		m.tab = tabNetwork
	case key.Matches(msg, k.Activity):
		m.tab, m.actOff = tabActivity, 0
	case key.Matches(msg, k.Pause):
		m.paused = !m.paused
		if !m.paused {
			m.feedOff = 0
		}
	case key.Matches(msg, k.Refresh):
		if !m.fetching {
			m.fetching = true
			return m, m.fetchSnapshot()
		}
	case key.Matches(msg, k.Next):
		m.focus = (m.focus + 1) % 3
	case key.Matches(msg, k.Prev):
		m.focus = (m.focus + 2) % 3
	case key.Matches(msg, k.Up):
		if m.tab == tabActivity {
			m.actOff = max(0, m.actOff-1)
			return m, nil
		}
		return m, m.moveSel(-1)
	case key.Matches(msg, k.Down):
		if m.tab == tabActivity {
			m.actOff++
			return m, nil
		}
		return m, m.moveSel(1)
	case key.Matches(msg, k.Top):
		return m, m.moveSel(-1 << 20)
	case key.Matches(msg, k.Bottom):
		return m, m.moveSel(1 << 20)
	case key.Matches(msg, k.Open):
		return m.open()
	}
	return m, nil
}

// open acts on the selection: an agent moves focus to its threads; a
// thread this host is part of opens as a conversation.
func (m model) open() (tea.Model, tea.Cmd) {
	switch m.focus {
	case paneAgents:
		m.focus = paneThreads
		return m, nil
	case paneThreads:
		ts := m.visibleThreads()
		if len(ts) == 0 {
			return m, nil
		}
		t := ts[min(m.threadSel, len(ts)-1)]
		if !t.Local {
			m.setFlash("that conversation is between other agents; only its state is shared")
			return m, nil
		}
		m.detail = &detail{peer: t.LocalPeer, th: t.Th}
		m.resizeDetail()
		return m, m.fetchThread(t.LocalPeer, t.Th)
	}
	return m, nil
}

func (m model) wheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	delta := 1
	if msg.Button == tea.MouseWheelUp {
		delta = -1
	}
	if m.detail != nil {
		if delta < 0 {
			m.detail.vp.ScrollUp(3)
		} else {
			m.detail.vp.ScrollDown(3)
		}
		return m, nil
	}
	if m.tab == tabActivity {
		m.actOff = max(0, m.actOff+delta)
		return m, nil
	}
	mouse := msg.Mouse()
	if p, ok := m.layout().paneAt(mouse.X, mouse.Y); ok {
		m.focus = p
	}
	return m, m.moveSel(delta)
}

func (m model) click(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.detail != nil || m.tab != tabDashboard || msg.Button != tea.MouseLeft {
		return m, nil
	}
	mouse := msg.Mouse()
	l := m.layout()
	p, ok := l.paneAt(mouse.X, mouse.Y)
	if !ok {
		return m, nil
	}
	m.focus = p
	r := l.rect(p)
	row := (mouse.Y - r.y - 1) / 2 // two lines per row, below the top border
	switch p {
	case paneAgents:
		if i := m.agentTop + row; i >= 0 && i < len(m.visibleAgents()) {
			m.agentSel = i
			m.threadSel, m.threadTop = 0, 0
			m.agentBar.target = float64(i)
		}
	case paneThreads:
		if i := m.threadTop + row; i >= 0 && i < len(m.visibleThreads()) {
			m.threadSel = i
			m.threadBar.target = float64(i)
		}
	}
	return m, tea.Batch(m.animate(), m.syncPreview())
}

// --- layout ---

type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type layout struct {
	agents, threads, feed, preview rect
	body                           rect
}

func (l layout) rect(p pane) rect {
	switch p {
	case paneAgents:
		return l.agents
	case paneThreads:
		return l.threads
	}
	return l.feed
}

func (l layout) paneAt(x, y int) (pane, bool) {
	for _, p := range []pane{paneAgents, paneThreads, paneFeed} {
		if l.rect(p).has(x, y) {
			return p, true
		}
	}
	return 0, false
}

const headerLines = 2

// layout places the panes. Wide: agents | threads over the conversation
// preview | activity. Medium: agents and threads on top, preview and
// activity below. Narrow: agents, threads and activity stacked.
func (m model) layout() layout {
	top := headerLines
	body := rect{0, top, m.w, max(6, m.h-top-m.footerHeight())}
	var l layout
	l.body = body
	switch {
	case m.w >= 130:
		aw := min(48, max(40, m.w*22/100))
		mw := (m.w - aw) * 52 / 100
		th := max(8, body.h*36/100)
		l.agents = rect{0, top, aw, body.h}
		l.threads = rect{aw, top, mw, th}
		l.preview = rect{aw, top + th, mw, body.h - th}
		l.feed = rect{aw + mw, top, m.w - aw - mw, body.h}
	case m.w >= 90:
		th := max(8, body.h*45/100)
		aw := 38
		l.agents = rect{0, top, aw, th}
		l.threads = rect{aw, top, m.w - aw, th}
		pw := m.w * 55 / 100
		l.preview = rect{0, top + th, pw, body.h - th}
		l.feed = rect{pw, top + th, m.w - pw, body.h - th}
	default:
		// Stacked, each list only as tall as it needs to be.
		ah := min(2*len(m.visibleAgents())+2, max(4, body.h*40/100))
		thh := min(max(4, 2*len(m.visibleThreads())+2), max(4, body.h*30/100))
		l.agents = rect{0, top, m.w, ah}
		l.threads = rect{0, top + ah, m.w, thh}
		l.feed = rect{0, top + ah + thh, m.w, max(3, body.h-ah-thh)}
	}
	return l
}

func (m model) footerHeight() int {
	if m.filtering {
		return 1
	}
	if m.help.ShowAll {
		return lipgloss.Height(m.help.View(m.keys))
	}
	return 1
}

func (m *model) resizeDetail() {
	d := m.detail
	if d == nil {
		return
	}
	h := max(3, m.h-headerLines-3-m.footerHeight())
	if d.vp.Width() == 0 {
		d.vp = viewport.New(viewport.WithWidth(m.w), viewport.WithHeight(h))
	} else {
		d.vp.SetWidth(m.w)
		d.vp.SetHeight(h)
	}
	m.renderDetail()
}

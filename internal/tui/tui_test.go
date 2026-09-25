package tui

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/store"
	"github.com/hollerprotocol/holler/wire"
)

func testKey(b byte) string {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = b
	return wire.FormatKey(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
}

var (
	bossKey    = testKey(1)
	workerKey  = testKey(2)
	builderKey = testKey(3)
)

const (
	fixSubject   = "Fix failing unit tests in calc"
	buildSubject = "Build release artifacts for v0.2.0"
)

// fixture is the network the tests watch from boss's host: boss is
// connected to worker, and builder is known only through worker's gossip.
// builderState is builder's state on its thread with worker.
func fixture(now time.Time, builderState string) *Snapshot {
	ts := wire.FormatTime(now.Add(-time.Minute))
	return &Snapshot{
		At: now,
		Status: &api.Status{
			Key: bossKey, Short: wire.ShortKey(bossKey), Name: "claude-code@boss", Version: "0.2.0", Presence: true, Unread: 1,
			Peers: []api.PeerView{{Key: workerKey, Short: wire.ShortKey(workerKey), Name: "claude-code@worker", Connected: true, LastSeen: now}},
		},
		Threads: []*store.Thread{{Peer: workerKey, Th: "thr_fix", Subject: fixSubject,
			MyState: wire.StateOpen, TheirState: wire.StateWaiting, Updated: now.Add(-time.Minute), Unread: 1}},
		Net: &api.Network{
			Self: api.AgentView{Presence: wire.Presence{Origin: bossKey, Name: "claude-code@boss"}, Status: api.AgentSelf, Sharing: true},
			Agents: []api.AgentView{
				{
					Presence: wire.Presence{Origin: workerKey, Name: "claude-code@worker", Version: "0.2.0", TS: ts,
						Peers: []wire.PresencePeer{{Key: bossKey, Name: "claude-code@boss", Up: true}, {Key: builderKey, Name: "codex@builder", Up: true}},
						Threads: []wire.PresenceThread{
							{Th: "thr_fix", Peer: bossKey, Subject: fixSubject, Mine: wire.StateWaiting, Theirs: wire.StateOpen, Updated: ts},
							{Th: "thr_build", Peer: builderKey, Subject: buildSubject, Mine: wire.StateOpen, Theirs: builderState, Updated: ts},
						}},
					Short: wire.ShortKey(workerKey), Status: api.AgentFresh, Direct: true, Via: workerKey,
				},
				{
					Presence: wire.Presence{Origin: builderKey, Name: "codex@builder", Version: "0.2.0", TS: ts,
						Peers: []wire.PresencePeer{{Key: workerKey, Name: "claude-code@worker", Up: true}},
						Threads: []wire.PresenceThread{
							{Th: "thr_build", Peer: workerKey, Subject: buildSubject, Mine: builderState, Theirs: wire.StateOpen, Updated: ts},
						}},
					Short: wire.ShortKey(builderKey), Status: api.AgentFresh, Via: workerKey, Hops: 1,
				},
			},
		},
	}
}

func msgEvent(seq int64, dir, text string, at time.Time) api.Event {
	id := fmt.Sprintf("01TESTMSG%017d", seq)
	raw, _ := json.Marshal(wire.Msg{Envelope: wire.Envelope{T: wire.TMsg, ID: id, TS: wire.FormatTime(at), Th: "thr_fix"},
		Parts: []wire.Part{{K: wire.PartText, Text: text}}})
	return api.Event{Seq: seq, At: at, Dir: dir, Type: wire.TMsg, Peer: workerKey, Th: "thr_fix", Subject: fixSubject, ID: id, Msg: raw, Acked: dir == "out"}
}

func stateEvent(seq int64, state, note string, at time.Time) api.Event {
	raw, _ := json.Marshal(wire.State{Envelope: wire.Envelope{T: wire.TState, TS: wire.FormatTime(at), Th: "thr_fix"}, State: state, Note: note})
	return api.Event{Seq: seq, At: at, Dir: "in", Type: wire.TState, Peer: workerKey, Th: "thr_fix", Subject: fixSubject, Msg: raw}
}

// conversation is the fix thread so far, as the daemon's log has it.
func conversation(now time.Time) []api.Event {
	return []api.Event{
		{Seq: 1, At: now.Add(-3 * time.Minute), Dir: "sys", Type: "connected", Peer: workerKey, Meta: map[string]any{"via": "tcp:127.0.0.1:47102"}},
		msgEvent(2, "out", "Please run the unit tests and fix every failure. Send the diff **before** changing files.", now.Add(-2*time.Minute)),
		stateEvent(3, wire.StateWorking, "running tests", now.Add(-110*time.Second)),
		{Seq: 4, At: now.Add(-100 * time.Second), Dir: "sys", Type: "presence", Peer: builderKey, Meta: map[string]any{"event": "joined", "via": workerKey}},
		msgEvent(5, "in", "5 tests, 2 errors. Proposed fix attached.", now.Add(-90*time.Second)),
		stateEvent(6, wire.StateWaiting, "awaiting diff approval", now.Add(-80*time.Second)),
	}
}

// fakeSource serves canned data in place of a daemon.
type fakeSource struct {
	mu      sync.Mutex
	snap    *Snapshot
	err     error
	history []api.Event
}

func (f *fakeSource) Snapshot(context.Context) (*Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap, f.err
}

func (f *fakeSource) History(context.Context, int) ([]api.Event, error) { return f.history, nil }

func (f *fakeSource) Subscribe(ctx context.Context, _ int64, _ func(api.Event)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeSource) Thread(context.Context, string, string) ([]api.Event, error) {
	var out []api.Event
	for _, ev := range f.history {
		if ev.Th == "thr_fix" {
			out = append(out, ev)
		}
	}
	return out, nil
}

// harness drives the model the way the Bubble Tea runtime would, one
// message at a time, without a terminal.
type harness struct {
	t   *testing.T
	src *fakeSource
	m   model
}

func newHarness(t *testing.T, w, h int) *harness {
	t.Helper()
	now := time.Now()
	src := &fakeSource{snap: fixture(now, wire.StateWorking), history: conversation(now)}
	hs := &harness{t: t, src: src, m: newModel(t.Context(), src, Options{})}
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.run(hs.m.fetchSnapshot())
	hs.run(hs.m.fetchHistory())
	if hs.m.snap == nil || !hs.m.historyLoaded {
		t.Fatal("harness did not load")
	}
	return hs
}

// send delivers one message and returns the command the model asked for.
func (h *harness) send(msg tea.Msg) tea.Cmd {
	m, cmd := h.m.Update(msg)
	h.m = m.(model)
	return cmd
}

// run executes a command and delivers its message.
func (h *harness) run(cmd tea.Cmd) {
	if cmd != nil {
		h.send(cmd())
	}
}

func (h *harness) press(keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = h.send(keyPress(k))
	}
	return cmd
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func (h *harness) screen() string {
	return ansi.Strip(h.m.View().Content)
}

func (h *harness) wantScreen(parts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			h.t.Errorf("screen lacks %q:\n%s", p, s)
			return
		}
	}
}

func TestBuildAgentsMergesPeersAndPresence(t *testing.T) {
	agents, names := buildAgents(fixture(time.Now(), wire.StateWorking))
	var got []string
	for _, a := range agents {
		got = append(got, a.Name+" "+a.Status)
	}
	want := []string{"claude-code@boss you", "claude-code@worker connected", "codex@builder online"}
	if !slices.Equal(got, want) {
		t.Fatalf("agents = %q, want %q", got, want)
	}
	me, worker, builder := agents[0], agents[1], agents[2]
	if len(me.Threads) != 1 || !me.Threads[0].Local || me.Threads[0].Theirs != wire.StateWaiting {
		t.Errorf("own threads = %+v", me.Threads)
	}
	// A peer's own presence replaces the local, one-sided view of it.
	if !worker.Direct || !worker.Presence || len(worker.Threads) != 2 {
		t.Errorf("worker = %+v", worker)
	}
	if builder.Direct || !builder.Presence || builder.Via != workerKey || builder.Hops != 1 {
		t.Errorf("builder route = direct %v presence %v via %q hops %d", builder.Direct, builder.Presence, builder.Via, builder.Hops)
	}
	if builder.count(wire.StateWorking) != 1 || builder.Threads[0].Local {
		t.Errorf("builder threads = %+v", builder.Threads)
	}
	if names[builderKey] != "codex@builder" || names[bossKey] != "claude-code@boss" {
		t.Errorf("names = %v", names)
	}
}

func TestDashboardShowsTheWholeNetwork(t *testing.T) {
	h := newHarness(t, 200, 50)
	h.wantScreen(
		"claude-code@boss", "sharing presence",
		"claude-code@worker", "connected",
		"codex@builder", "online", "via claude-code@worker",
		fixSubject, "1 working", "1 waiting",
		// The feed, from the daemon's log.
		"connected via tcp:127.0.0.1:47102",
		"joined the network via claude-code@worker",
		"is waiting · awaiting diff approval",
		// The selected thread's conversation, previewed.
		"Conversation · "+fixSubject, "5 tests, 2 errors",
	)
	// Snippets drop markdown emphasis.
	if s := h.screen(); strings.Contains(s, "**before**") {
		t.Errorf("snippet kept markdown:\n%s", s)
	}
}

func TestEveryViewFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{200, 50}, {120, 40}, {100, 30}, {80, 24}, {64, 20}} {
		w, hh := size[0], size[1]
		h := newHarness(t, w, hh)
		check := func(view string) {
			t.Helper()
			lines := strings.Split(h.screen(), "\n")
			if len(lines) > hh {
				t.Errorf("%dx%d %s: %d lines", w, hh, view, len(lines))
			}
			for i, l := range lines {
				if n := ansi.StringWidth(l); n > w {
					t.Errorf("%dx%d %s: line %d is %d wide: %q", w, hh, view, i, n, l)
				}
			}
		}
		check("dashboard")
		h.press("2")
		check("network")
		h.press("3")
		check("activity")
		h.press("1", "tab")
		h.run(h.press("enter"))
		if h.m.detail == nil {
			t.Fatalf("%dx%d: enter did not open the thread", w, hh)
		}
		check("thread")
	}
}

// Long subjects and names are cut to fit, never pushing panes off screen.
func TestLongNamesFit(t *testing.T) {
	long := strings.Repeat("Run the integration suite on the release branch and report which tests flake ", 2)
	host := "a-build-machine-with-a-very-long-hostname.internal.example.com"
	for _, size := range [][2]int{{250, 60}, {180, 48}, {130, 40}, {100, 30}, {80, 24}} {
		w, hh := size[0], size[1]
		h := newHarness(t, w, hh)
		snap := h.src.snap
		snap.Status.Name = "claude-code@" + host
		snap.Status.Peers[0].Name = "claude-code@worker." + host
		snap.Threads[0].Subject = long
		for i := range snap.Net.Agents {
			a := &snap.Net.Agents[i]
			a.Presence.Name += "." + host
			for j := range a.Presence.Threads {
				a.Presence.Threads[j].Subject = long
			}
		}
		h.run(h.m.fetchSnapshot())
		h.run(h.m.fetchThread(workerKey, "thr_fix"))
		check := func(view string) {
			t.Helper()
			lines := strings.Split(h.screen(), "\n")
			if len(lines) > hh {
				t.Errorf("%dx%d %s: %d lines", w, hh, view, len(lines))
			}
			for i, l := range lines {
				if n := ansi.StringWidth(l); n > w {
					t.Errorf("%dx%d %s: line %d is %d wide: %q", w, hh, view, i, n, ansi.Strip(l))
				}
			}
		}
		check("dashboard, local thread")
		h.press("j", "j")
		check("dashboard, remote thread")
		h.press("2")
		check("network")
		h.press("3")
		check("activity")
	}
}

func TestTruncate(t *testing.T) {
	for _, c := range []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 4, "abc…"},
		{"tailcat:ab…xyz", 12, "tailcat:ab…"},
		{"tailcat:ab…xyz", 13, "tailcat:ab…x…"},
	} {
		if got := ansi.Strip(truncate(c.in, c.w)); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
		styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87")).Render(c.in)
		if got := ansi.Strip(truncate(styled, c.w)); got != c.want {
			t.Errorf("truncate(styled %q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

// Pasted output keeps its lines instead of being reflowed into one.
func TestMarkdownKeepsLines(t *testing.T) {
	var md markdown
	out := ansi.Strip(md.render("k", "Ran the suite:\ntest_add ... ok\ntest_average ... ok", 60, true))
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if want := []string{"Ran the suite:", "test_add ... ok", "test_average ... ok"}; !slices.Equal(lines, want) {
		t.Errorf("rendered lines %q, want %q", lines, want)
	}
}

func TestPreviewFollowsTheSelectedThread(t *testing.T) {
	h := newHarness(t, 120, 40)
	if h.m.layout().preview.w == 0 {
		t.Fatal("no preview at 120 columns")
	}
	p := h.m.preview
	if p == nil || p.th != "thr_fix" || p.peer != workerKey {
		t.Fatalf("preview = %+v", p)
	}
	h.run(h.m.fetchThread(workerKey, "thr_fix"))
	if !p.loaded || len(p.events) != 4 {
		t.Fatalf("preview loaded %v with %d events", p.loaded, len(p.events))
	}
	h.wantScreen("── claude-code@worker is ◐ waiting · awaiting diff approval")
	h.send(eventMsg{msgEvent(7, "in", "Applied. Ran 5 tests, OK.", time.Now())})
	h.send(eventMsg{stateEvent(8, wire.StateDone, "", time.Now())})
	if len(p.events) != 6 {
		t.Errorf("preview has %d events after two more", len(p.events))
	}
	h.wantScreen("── claude-code@worker is ✔ done")

	// A thread between other agents has no conversation to show.
	h.press("j", "j")
	if h.m.preview != nil {
		t.Errorf("preview kept for a remote thread: %+v", h.m.preview)
	}
}

func TestNoPreviewWhenNarrow(t *testing.T) {
	h := newHarness(t, 70, 30)
	if h.m.layout().preview.w != 0 {
		t.Error("preview at 70 columns")
	}
	if strings.Contains(h.screen(), "Conversation") {
		t.Errorf("narrow screen shows the preview:\n%s", h.screen())
	}
}

func TestTabs(t *testing.T) {
	h := newHarness(t, 160, 40)
	h.press("2")
	if h.m.tab != tabNetwork {
		t.Fatalf("tab = %v", h.m.tab)
	}
	// builder hangs off worker in the tree: that is how boss hears of it.
	h.wantScreen("◆ claude-code@boss", "╰──● claude-code@worker", "╰──● codex@builder", "Threads in flight", buildSubject)
	h.press("3")
	h.wantScreen("TIME", "KIND", "joined", "Please run the unit tests")
	h.press("esc")
	if h.m.tab != tabDashboard {
		t.Errorf("esc left tab %v", h.m.tab)
	}
}

func TestOpenAThreadThisHostIsIn(t *testing.T) {
	h := newHarness(t, 120, 40)
	h.press("tab")
	cmd := h.press("enter")
	if h.m.detail == nil {
		t.Fatal("no detail")
	}
	h.run(cmd)
	h.wantScreen(fixSubject, "Please run the unit tests", "5 tests, 2 errors", "claude-code@worker is ◐ waiting · awaiting diff approval")

	// Live: a new message lands in the open conversation.
	h.send(eventMsg{msgEvent(7, "in", "Applied. Ran 5 tests, OK.", time.Now())})
	h.wantScreen("Applied. Ran 5 tests, OK.")

	h.press("esc")
	if h.m.detail != nil {
		t.Error("esc did not close the thread")
	}
}

func TestOtherAgentsConversationsStayPrivate(t *testing.T) {
	h := newHarness(t, 120, 40)
	h.press("j", "j") // builder
	if a := h.m.selectedAgent(); a == nil || a.Key != builderKey {
		t.Fatalf("selected %+v", a)
	}
	h.wantScreen("Threads · codex@builder", buildSubject, "Presence shares only its subject and states.")
	h.press("tab", "enter")
	if h.m.detail != nil {
		t.Fatal("opened a thread this host is not part of")
	}
	if !strings.Contains(h.m.flash, "between other agents") {
		t.Errorf("flash = %q", h.m.flash)
	}
}

func TestFilter(t *testing.T) {
	h := newHarness(t, 160, 40)
	h.press("/", "b", "u", "i", "l", "d")
	if !h.m.filtering || h.m.filter.Value() != "build" {
		t.Fatalf("filtering %v value %q", h.m.filtering, h.m.filter.Value())
	}
	var names []string
	for _, a := range h.m.visibleAgents() {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []string{"claude-code@worker", "codex@builder"}) {
		t.Errorf("filtered agents = %q", names)
	}
	for _, it := range h.m.visibleFeed() {
		if !strings.Contains(it.haystack(h.m.names), "build") {
			t.Errorf("feed kept %+v", it)
		}
	}
	h.press("enter")
	h.wantScreen("filter: build")
	h.press("esc")
	if h.m.filter.Value() != "" || len(h.m.visibleAgents()) != 3 {
		t.Errorf("esc kept filter %q", h.m.filter.Value())
	}
}

func TestLiveEventsReachTheFeedOnce(t *testing.T) {
	h := newHarness(t, 200, 50)
	n := len(h.m.feed)
	ev := stateEvent(7, wire.StateWorking, "applying the fix", time.Now())
	h.send(eventMsg{ev})
	h.send(eventMsg{ev}) // the stream may replay after a reconnect
	if len(h.m.feed) != n+1 {
		t.Fatalf("feed grew by %d", len(h.m.feed)-n)
	}
	if it := h.m.feed[len(h.m.feed)-1]; it.Born.IsZero() || it.Text != "working: applying the fix" {
		t.Errorf("item = %+v", it)
	}
	h.wantScreen("is working · applying the fix")
}

func TestPresenceChangesReachTheFeed(t *testing.T) {
	h := newHarness(t, 200, 50)
	h.src.mu.Lock()
	h.src.snap = fixture(time.Now(), wire.StateDone)
	h.src.mu.Unlock()
	h.run(h.m.fetchSnapshot())
	it := h.m.feed[len(h.m.feed)-1]
	if it.Kind != "remote" || it.From != builderKey || it.To != workerKey || it.Text != "is now done" {
		t.Fatalf("item = %+v", it)
	}
	h.wantScreen("codex@builder  is now done with claude-code@worker")
}

func TestNewRemoteThreadAnnouncedOnce(t *testing.T) {
	now := time.Now()
	before := fixture(now, wire.StateOpen)
	for i := range before.Net.Agents {
		a := &before.Net.Agents[i]
		a.Threads = slices.DeleteFunc(a.Threads, func(t wire.PresenceThread) bool { return t.Th == "thr_build" })
	}
	prev, _ := buildAgents(before)
	cur, _ := buildAgents(fixture(now, wire.StateOpen))
	items := diffPresence(prev, cur, bossKey, now)
	if len(items) != 1 || items[0].Text != "opened a thread" || items[0].Th != "thr_build" {
		t.Fatalf("items = %+v", items)
	}
	// Seen from the other side a moment later: still one.
	if again := diffPresence(cur, cur, bossKey, now); len(again) != 0 {
		t.Fatalf("repeated items = %+v", again)
	}
}

func TestOfflineCard(t *testing.T) {
	src := &fakeSource{err: errors.New("dial unix: no such file or directory")}
	h := &harness{t: t, src: src, m: newModel(t.Context(), src, Options{})}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.run(h.m.fetchSnapshot())
	h.wantScreen("No holler daemon on this host yet", "holler up --presence")
}

func TestDaemonLostKeepsTheLastPicture(t *testing.T) {
	h := newHarness(t, 160, 40)
	h.src.mu.Lock()
	h.src.snap, h.src.err = nil, errors.New("connection refused")
	h.src.mu.Unlock()
	h.run(h.m.fetchSnapshot())
	h.wantScreen("daemon unreachable", "codex@builder", fixSubject)
}

func TestAckPending(t *testing.T) {
	now := time.Now()
	sent := msgEvent(1, "out", "hi", now.Add(-time.Second))
	sent.Acked = false
	if !ackPending([]api.Event{sent}, now) {
		t.Error("fresh unacked message not pending")
	}
	if ackPending([]api.Event{sent}, now.Add(time.Minute)) {
		t.Error("old unacked message still pending")
	}
	sent.Acked = true
	if ackPending([]api.Event{sent, msgEvent(2, "in", "yo", now)}, now) {
		t.Error("acked message pending")
	}
}

package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hollerprotocol/holler/wire"
)

// View implements tea.Model.
func (m model) View() tea.View {
	var body string
	switch {
	case m.w == 0:
		body = ""
	case m.snap == nil && m.snapErr != nil:
		body = m.header() + "\n" + m.offline()
	case m.snap == nil:
		body = m.header() + "\n" + m.center(m.spin.View()+" connecting to the holler daemon…", m.h-headerLines)
	case m.detail != nil:
		body = m.header() + "\n" + m.detailView() + "\n" + m.footer()
	case m.tab == tabNetwork:
		body = m.header() + "\n" + m.fit(m.networkView(), m.layout().body.h) + "\n" + m.footer()
	case m.tab == tabActivity:
		body = m.header() + "\n" + m.fit(m.activityView(), m.layout().body.h) + "\n" + m.footer()
	default:
		body = m.header() + "\n" + m.dashboard() + "\n" + m.footer()
	}
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "holler watch"
	if m.snap != nil && m.snap.Status != nil {
		v.WindowTitle = "holler watch · " + m.snap.Status.Name
	}
	return v
}

// fit pads or cuts s to exactly h lines of width m.w.
func (m model) fit(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.w, "…")
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) center(s string, h int) string {
	return lipgloss.Place(m.w, max(1, h), lipgloss.Center, lipgloss.Center, s)
}

// --- header ---

func (m model) header() string {
	t := m.th
	// Narrow terminals drop the least useful parts first.
	var line1 string
	for level := 0; level <= 5; level++ {
		left, right := m.headerParts(level)
		line1 = m.spread(left, right)
		if lipgloss.Width(left)+lipgloss.Width(right) < m.w {
			break
		}
	}

	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if tab(i) == m.tab && m.detail == nil {
			tabs = append(tabs, lipgloss.NewStyle().Foreground(t.accent).Bold(true).Underline(true).Render(label))
		} else {
			tabs = append(tabs, t.dimText.Render(label))
		}
	}
	var status string
	switch {
	case m.snapErr != nil && m.snap != nil:
		status = lipgloss.NewStyle().Foreground(t.bad).Render("● daemon unreachable, retrying")
	case m.flash != "" && time.Now().Before(m.flashTill):
		status = lipgloss.NewStyle().Foreground(t.info).Render(m.flash)
	case m.filter.Value() != "":
		status = t.accentText.Render("filter: ") + t.text.Render(m.filter.Value())
	}
	if m.paused {
		status = lipgloss.NewStyle().Foreground(t.warn).Render("⏸ feed paused  ") + status
	}
	line2 := m.spread(" "+strings.Join(tabs, ""), status+" ")
	return line1 + "\n" + line2
}

// headerParts is the top line's two halves. Each level leaves out one more
// thing: the sparkline, the presence label, the short key, the thread count
// and the count of agents up.
func (m model) headerParts(level int) (string, string) {
	t := m.th
	left := gradient(" holler ", t.accent2, t.accent) + t.dimText.Render("watch")
	var right []string
	if m.snap != nil && m.snap.Status != nil {
		st := m.snap.Status
		left += "  " + lipgloss.NewStyle().Foreground(t.agentColor(st.Key)).Bold(true).Render(st.Name)
		if level < 3 {
			left += t.faintText.Render(" " + st.Short)
		}
		switch {
		case st.Presence && level < 2:
			left += " " + lipgloss.NewStyle().Foreground(t.good).Render("◉ sharing presence")
		case st.Presence:
			left += " " + lipgloss.NewStyle().Foreground(t.good).Render("◉")
		case level < 2:
			left += " " + t.faintText.Render("○ presence off")
		default:
			left += " " + t.faintText.Render("○")
		}
		online, threads, working, waiting := 0, 0, 0, 0
		seen := map[string]bool{}
		for _, a := range m.agents {
			if a.Status == stConnected || a.Status == stOnline || a.Self {
				online++
			}
			for _, th := range a.Threads {
				if !seen[th.Th] {
					seen[th.Th] = true
					threads++
				}
			}
			working += a.count(wire.StateWorking)
			waiting += a.count(wire.StateWaiting)
		}
		agents := t.dimText.Render(plural(len(m.agents), "agent"))
		if level < 5 {
			agents += t.faintText.Render(fmt.Sprintf(" (%d up)", online))
		}
		right = append(right, agents)
		if level < 4 {
			right = append(right, t.dimText.Render(plural(threads, "thread")))
		}
		if working > 0 {
			right = append(right, m.spin.View()+lipgloss.NewStyle().Foreground(t.warn).Render(fmt.Sprintf(" %d working", working)))
		}
		if waiting > 0 {
			right = append(right, lipgloss.NewStyle().Foreground(t.accent2).Render(fmt.Sprintf("◐ %d waiting", waiting)))
		}
	}
	if level < 1 {
		right = append(right, m.sparkline(12))
	}
	right = append(right, m.pulseDot()+" "+t.text.Render(m.now.Format("15:04:05")))
	return left, strings.Join(right, t.faintText.Render("  ·  ")) + " "
}

// spread puts left and right on one line of width m.w.
func (m model) spread(left, right string) string {
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left+" "+right, m.w, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

// sparkline shows messages per minute over the last n minutes.
func (m model) sparkline(n int) string {
	buckets := make([]int, n)
	now := m.now
	for i := len(m.feed) - 1; i >= 0; i-- {
		it := m.feed[i]
		age := now.Sub(it.At)
		if age >= time.Duration(n)*time.Minute {
			break
		}
		if age < 0 {
			age = 0
		}
		if it.Kind == wire.TMsg || it.Kind == wire.TState || it.Kind == "remote" {
			buckets[n-1-int(age/time.Minute)]++
		}
	}
	peak := 0
	for _, b := range buckets {
		peak = max(peak, b)
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	var s strings.Builder
	colors := lipgloss.Blend1D(n, m.th.accent, m.th.accent2)
	for i, b := range buckets {
		r := bars[0]
		if peak > 0 && b > 0 {
			r = bars[min(len(bars)-1, 1+b*(len(bars)-2)/peak)]
		}
		s.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Render(string(r)))
	}
	return s.String()
}

// pulseDot flashes when an event arrives and fades with a spring.
func (m model) pulseDot() string {
	p := math.Max(0, math.Min(1, m.pulse.pos))
	return lipgloss.NewStyle().Foreground(blend(m.th.faint, m.th.accent2, p)).Render("●")
}

// --- panes ---

// box draws a rounded pane of exactly w×h with a title in its top edge.
func (m model) box(title string, count int, lines []string, w, h int, focused bool) string {
	t := m.th
	bc := t.border
	titleStyle := t.dimText.Bold(true)
	if focused {
		bc = t.accent
		titleStyle = t.accentText
	}
	b := lipgloss.NewStyle().Foreground(bc)
	label := titleStyle.Render(title)
	if count >= 0 {
		label += t.faintText.Render(fmt.Sprintf(" %d", count))
	}
	label = " " + label + " "
	inner := max(1, w-4)
	fill := max(0, w-3-lipgloss.Width(label))
	var out strings.Builder
	out.WriteString(b.Render("╭─") + label + b.Render(strings.Repeat("─", fill)+"╮") + "\n")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], inner, "…")
		}
		pad := max(0, inner-lipgloss.Width(line))
		out.WriteString(b.Render("│") + " " + line + strings.Repeat(" ", pad) + " " + b.Render("│") + "\n")
	}
	out.WriteString(b.Render("╰" + strings.Repeat("─", max(0, w-2)) + "╯"))
	return out.String()
}

func (m model) dashboard() string {
	l := m.layout()
	agents := m.agentsPane(l.agents)
	threads := m.threadsPane(l.threads)
	feed := m.feedPane(l.feed)
	switch {
	case m.w >= 130:
		middle := lipgloss.JoinVertical(lipgloss.Left, threads, m.previewPane(l.preview))
		return lipgloss.JoinHorizontal(lipgloss.Top, agents, middle, feed)
	case m.w >= 90:
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, agents, threads),
			lipgloss.JoinHorizontal(lipgloss.Top, m.previewPane(l.preview), feed))
	}
	return lipgloss.JoinVertical(lipgloss.Left, agents, threads, feed)
}

// scrollTop keeps the selected row (of rowLines lines each) visible.
func scrollTop(top, sel, rows int) int {
	if rows <= 0 {
		return 0
	}
	if sel < top {
		return sel
	}
	if sel >= top+rows {
		return sel - rows + 1
	}
	return top
}

// bar is the selection marker, two lines tall. Its position is a spring,
// so it slides from row to row instead of jumping.
func (m model) bar(sp spring, row, line int) string {
	pos := sp.pos * 2 // two lines per row
	idx := float64(row*2 + line)
	if idx >= pos-0.5 && idx < pos+1.5 {
		return lipgloss.NewStyle().Foreground(m.th.accent).Render("▌")
	}
	return " "
}

func (m model) statusDot(a agentRow) string {
	t := m.th
	c, glyph := t.faint, "○"
	switch a.Status {
	case stYou:
		c, glyph = t.accent, "◆"
	case stConnected:
		c, glyph = t.good, "●"
	case stOnline:
		c, glyph = t.info, "●"
	case stReconnecting:
		c, glyph = t.warn, "◌"
	case stStale:
		c, glyph = t.warn, "◍"
	case stBye:
		c, glyph = t.faint, "◌"
	}
	return lipgloss.NewStyle().Foreground(c).Render(glyph)
}

func (m model) agentsPane(r rect) string {
	t := m.th
	as := m.visibleAgents()
	rows := (r.h - 2) / 2
	top := scrollTop(m.agentTop, m.agentSel, rows)
	var lines []string
	inner := r.w - 4
	for i := top; i < len(as) && i < top+rows; i++ {
		a := as[i]
		name := lipgloss.NewStyle().Foreground(t.agentColor(a.Key)).Bold(i == m.agentSel && m.focus == paneAgents).Render(a.Name)
		if a.Self {
			name += t.faintText.Render(" (you)")
		}
		status := t.dimText.Render(a.Status)
		first := m.bar(m.agentBar, i, 0) + m.statusDot(a) + " " + name
		first = m.spreadIn(first, status, inner)
		// Most telling first: truncation eats the key, not the activity.
		var bits []string
		switch {
		case a.count(wire.StateWorking) > 0:
			bits = append(bits, m.spin.View()+lipgloss.NewStyle().Foreground(t.warn).Render(" working"))
		case a.count(wire.StateWaiting) > 0:
			bits = append(bits, lipgloss.NewStyle().Foreground(t.accent2).Render("◐ waiting"))
		case a.active() > 0:
			bits = append(bits, t.dimText.Render(fmt.Sprintf("%d active", a.active())))
		case len(a.Threads) > 0:
			bits = append(bits, t.faintText.Render(plural(len(a.Threads), "thread")))
		}
		if a.Unread > 0 && a.Self {
			bits = append(bits, lipgloss.NewStyle().Foreground(t.accent2).Render(fmt.Sprintf("✉ %d", a.Unread)))
		}
		if !a.Self && !a.Direct && a.Via != "" {
			bits = append(bits, t.faintText.Render("via "+m.nameOf(a.Via)))
		}
		bits = append(bits, t.faintText.Render(a.Short))
		second := m.bar(m.agentBar, i, 1) + "  " + strings.Join(bits, t.faintText.Render(" · "))
		lines = append(lines, first, second)
	}
	if len(as) <= 1 && m.query() == "" {
		lines = append(lines, "", t.faintText.Render("  no peers yet"), t.faintText.Render("  holler connect <address>"))
	}
	// The selected agent's details fill the room left under the list.
	if a := m.selectedAgent(); a != nil {
		card := m.agentCard(*a, inner)
		if room := r.h - 2 - len(lines); room > len(card) {
			for len(lines) < r.h-2-len(card) {
				lines = append(lines, "")
			}
			lines = append(lines, card...)
		}
	}
	return m.box("Agents", len(as), lines, r.w, r.h, m.focus == paneAgents)
}

// agentCard describes one agent: who it is, how this host hears of it,
// and whom it is connected to.
func (m model) agentCard(a agentRow, width int) []string {
	t := m.th
	who := func(key string) string {
		return lipgloss.NewStyle().Foreground(t.agentColor(key)).Render(m.nameOf(key))
	}
	row := func(k, v string) string {
		return t.faintText.Render(fmt.Sprintf(" %-9s", k)) + v
	}
	name := lipgloss.NewStyle().Foreground(t.agentColor(a.Key)).Bold(true).Render(a.Name)
	out := []string{t.faintText.Render("── ") + name + " " + t.faintText.Render(strings.Repeat("─", max(0, width-lipgloss.Width(a.Name)-4)))}
	// The daemon's default about line only repeats the version and key.
	if a.About != "" && !strings.HasPrefix(a.About, "holler-go ") {
		out = append(out, row("about", t.text.Render(a.About)))
	}
	out = append(out, row("key", t.dimText.Render(a.Key)))
	if a.Version != "" {
		out = append(out, row("version", t.dimText.Render(a.Version)))
	}
	status := m.statusDot(a) + " " + t.text.Render(a.Status)
	if !a.Self && !a.Seen.IsZero() {
		status += t.faintText.Render(" · seen " + ago(m.now, a.Seen))
	}
	out = append(out, row("status", status))
	switch {
	case a.Self && a.Presence:
		out = append(out, row("presence", lipgloss.NewStyle().Foreground(t.good).Render("shared with the network")))
	case a.Self:
		out = append(out, row("presence", t.dimText.Render("off · holler up --presence")))
	case a.Direct:
		out = append(out, row("route", t.dimText.Render("direct peer of this host")))
	case a.Via != "":
		route := t.dimText.Render("via ") + who(a.Via)
		if a.Hops > 1 {
			route += t.faintText.Render(fmt.Sprintf(" · %d relays", a.Hops))
		}
		out = append(out, row("route", route))
	}
	if len(a.Threads) > 0 {
		out = append(out, row("threads", t.dimText.Render(fmt.Sprintf("%d · %d active", len(a.Threads), a.active()))))
	}
	for i, p := range a.Peers {
		if i == 4 {
			out = append(out, row("", t.faintText.Render(fmt.Sprintf("+%d more", len(a.Peers)-4))))
			break
		}
		dot := t.faintText.Render("○")
		if p.Up {
			dot = lipgloss.NewStyle().Foreground(t.good).Render("●")
		}
		k := ""
		if i == 0 {
			k = "peers"
		}
		out = append(out, row(k, dot+" "+who(p.Key)))
	}
	return out
}

// spreadIn right-aligns right within width.
func (m model) spreadIn(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// plural is "1 thread", "2 threads".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (m model) nameOf(key string) string {
	if n := m.names[key]; n != "" {
		return n
	}
	return wire.ShortKey(key)
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < 5*time.Second:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return t.Format("Jan 2")
}

func (m model) threadsPane(r rect) string {
	t := m.th
	a := m.selectedAgent()
	title := "Threads"
	if a != nil {
		title = "Threads · " + a.Name
	}
	ts := m.visibleThreads()
	rows := (r.h - 2) / 2
	top := scrollTop(m.threadTop, m.threadSel, rows)
	inner := r.w - 4
	var lines []string
	for i := top; i < len(ts) && i < top+rows; i++ {
		th := ts[i]
		subject := th.Subject
		if subject == "" {
			subject = th.Th
		}
		subj := t.text.Bold(i == m.threadSel && m.focus == paneThreads).Render(subject)
		first := m.spreadIn(m.bar(m.threadBar, i, 0)+" "+subj, t.faintText.Render(ago(m.now, th.Updated)), inner)
		// This agent's state, the other party, and theirs.
		peer := lipgloss.NewStyle().Foreground(t.agentColor(th.Peer)).Render(th.PeerName)
		second := m.bar(m.threadBar, i, 1) + "  " + t.badge(th.Mine) + t.faintText.Render("  ⇄  ") + peer + " " + t.badge(th.Theirs)
		if th.Unread > 0 {
			second += "  " + lipgloss.NewStyle().Foreground(t.accent2).Render(fmt.Sprintf("✉ %d", th.Unread))
		}
		if th.Local && i == m.threadSel && m.focus == paneThreads {
			second += "  " + t.faintText.Render("↵ open")
		}
		lines = append(lines, first, second)
	}
	if len(ts) == 0 {
		msg := "no threads"
		if a != nil && !a.Self && !a.Presence && !a.Direct {
			msg = "this agent shares no presence"
		}
		lines = append(lines, "", t.faintText.Render("  "+msg))
	}
	return m.box(title, len(ts), lines, r.w, r.h, m.focus == paneThreads)
}

// feedLine renders one activity item as a sentence: a glyph for its kind,
// who, and what happened.
func (m model) feedLine(it feedItem, width int, withTime bool) string {
	t := m.th
	who := func(key string) string {
		return lipgloss.NewStyle().Foreground(t.agentColor(key)).Render(m.nameOf(key))
	}
	dim := t.dimText.Render
	note := func(s string) string {
		if s == "" {
			return ""
		}
		return dim(" · " + s)
	}
	glyph, gc := "·", t.dim
	pair := it.To != "" // "from → to"
	text := t.text.Render(it.Text)
	switch it.Kind {
	case wire.TMsg:
		glyph, gc = "✉", t.fg
	case wire.TState:
		state, rest, _ := strings.Cut(it.Text, ": ")
		glyph, gc, pair = "◆", t.stateColor(state), false
		text = dim("is ") + lipgloss.NewStyle().Foreground(gc).Render(state) + note(rest)
	case "remote":
		glyph, gc, pair = "✚", t.info, false
		text = dim(it.Text)
		if state, ok := strings.CutPrefix(it.Text, "is now "); ok {
			glyph, gc = "◆", t.stateColor(state)
			text = dim("is now ") + lipgloss.NewStyle().Foreground(gc).Render(state)
		}
		if it.To != "" {
			text += dim(" with ") + who(it.To)
		}
	case "connect":
		glyph, gc, pair = "⚡", t.good, false
		text = dim("connected " + it.Text)
	case "disconnect":
		glyph, gc, pair = "✕", t.faint, false
		text = dim("disconnected") + note(it.Text)
	case "joined":
		glyph, gc, pair = "⚡", t.good, false
		text = dim(it.Text)
		if it.Via != "" {
			text += dim(" via ") + who(it.Via)
		}
	case "left":
		glyph, gc, pair = "✕", t.faint, false
		text = dim(it.Text)
	case "blob":
		glyph, gc = "⬇", t.info
	case wire.TGrant:
		glyph, gc = "⚷", t.warn
		text = dim("granted ") + t.text.Render(it.Text)
	case wire.TIntroduce:
		glyph, gc = "✚", t.info
	case wire.TBye:
		glyph, gc, pair = "✕", t.faint, false
		text = dim("said bye") + note(it.Text)
	case "refused":
		glyph, gc, pair = "!", t.bad, false
		text = lipgloss.NewStyle().Foreground(t.bad).Render("was refused") + note(it.Text)
	case wire.TErr:
		glyph, gc = "!", t.bad
		text = lipgloss.NewStyle().Foreground(t.bad).Render(it.Text)
	}
	var b strings.Builder
	if withTime {
		b.WriteString(t.faintText.Render(it.At.Local().Format("15:04:05")) + " ")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(gc).Render(glyph) + " " + who(it.From))
	if pair {
		b.WriteString(t.faintText.Render(" → ") + who(it.To))
	}
	if it.Text != "" || !pair {
		b.WriteString("  " + text)
	}
	if it.Subject != "" {
		b.WriteString(t.faintText.Render("  · " + it.Subject))
	}
	line := b.String()
	// New lines glow, then settle.
	if !it.Born.IsZero() {
		if age := time.Since(it.Born); age < freshFor {
			glow := 1 - float64(age)/float64(freshFor)
			mark := lipgloss.NewStyle().Foreground(blend(t.faint, t.accent2, glow)).Render("▍")
			line = mark + line
		} else {
			line = " " + line
		}
	} else {
		line = " " + line
	}
	return ansi.Truncate(line, width, "…")
}

func (m model) feedPane(r rect) string {
	items := m.visibleFeed()
	rows := r.h - 2
	end := max(0, len(items)-m.feedOff)
	start := max(0, end-rows)
	var lines []string
	for _, it := range items[start:end] {
		lines = append(lines, m.feedLine(it, r.w-4, true))
	}
	if len(items) == 0 {
		lines = append(lines, "", m.th.faintText.Render("  waiting for activity…"))
	}
	title := "Activity"
	if m.paused {
		title += " (paused)"
	}
	return m.box(title, len(items), lines, r.w, r.h, m.focus == paneFeed)
}

// --- footer and empty states ---

func (m model) footer() string {
	if m.filtering {
		return m.filter.View()
	}
	if m.detail != nil {
		return m.help.ShortHelpView([]key.Binding{m.keys.Up, m.keys.Down, m.keys.Back, m.keys.Refresh, m.keys.Quit})
	}
	return m.help.View(m.keys)
}

func (m model) offline() string {
	t := m.th
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.accent).
		Padding(1, 3).
		Render(t.accentText.Render("No holler daemon on this host yet") + "\n\n" +
			t.text.Render("Start one, sharing presence so other hosts can watch it:") + "\n\n" +
			lipgloss.NewStyle().Foreground(t.accent2).Render("  holler up --presence") + "\n\n" +
			t.dimText.Render(m.spin.View()+" retrying every few seconds · q to quit"))
	return m.center(card, m.h-headerLines)
}

package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"charm.land/lipgloss/v2/tree"

	"github.com/hollerprotocol/holler/wire"
)

// networkView draws the network as a tree rooted at this agent: its peers,
// then theirs, as far as presence reaches, plus every thread in flight.
func (m model) networkView() string {
	t := m.th
	if len(m.agents) == 0 {
		return ""
	}
	byKey := map[string]agentRow{}
	for _, a := range m.agents {
		byKey[a.Key] = a
	}
	label := func(key string) string {
		a, ok := byKey[key]
		if !ok {
			return t.faintText.Render("○ ") + lipgloss.NewStyle().Foreground(t.agentColor(key)).Render(m.nameOf(key)) + t.faintText.Render(" (no presence)")
		}
		s := m.statusDot(a) + " " + lipgloss.NewStyle().Foreground(t.agentColor(a.Key)).Bold(true).Render(a.Name)
		s += " " + t.faintText.Render(a.Short) + "  " + t.dimText.Render(a.Status)
		if w := a.count(wire.StateWorking); w > 0 {
			s += "  " + m.spin.View() + lipgloss.NewStyle().Foreground(t.warn).Render(fmt.Sprintf(" %d working", w))
		}
		if w := a.count(wire.StateWaiting); w > 0 {
			s += "  " + lipgloss.NewStyle().Foreground(t.accent2).Render(fmt.Sprintf("◐ %d waiting", w))
		}
		if n := a.active(); n > 0 {
			s += "  " + t.dimText.Render(plural(n, "active thread"))
		}
		return s
	}
	// Breadth-first from this agent, so each agent sits under the peer
	// closest to us.
	visited := map[string]bool{m.self: true}
	root := tree.Root(label(m.self)).
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(lipgloss.NewStyle().Foreground(t.border)).
		IndenterStyle(lipgloss.NewStyle().Foreground(t.border))
	type item struct {
		key string
		t   *tree.Tree
	}
	queue := []item{{m.self, root}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		var kids []string
		for _, p := range byKey[cur.key].Peers {
			if !visited[p.Key] {
				visited[p.Key] = true
				kids = append(kids, p.Key)
			}
		}
		sort.SliceStable(kids, func(i, j int) bool {
			return statusRank[byKey[kids[i]].Status] < statusRank[byKey[kids[j]].Status]
		})
		for _, k := range kids {
			child := tree.Root(label(k))
			cur.t.Child(child)
			queue = append(queue, item{k, child})
		}
	}
	// Agents heard of but not linked to anyone we can see.
	var loose []string
	for _, a := range m.agents {
		if !visited[a.Key] {
			loose = append(loose, a.Key)
		}
	}
	var b strings.Builder
	b.WriteString("\n" + t.accentText.Render("  Network") + t.faintText.Render("  who is connected to whom, as far as presence reaches") + "\n\n")
	for _, l := range strings.Split(root.String(), "\n") {
		b.WriteString("  " + l + "\n")
	}
	for _, k := range loose {
		b.WriteString("  " + label(k) + t.faintText.Render("  (not linked)") + "\n")
	}

	// Every thread, once, with both sides' states.
	seen := map[string]bool{}
	var rows [][]string
	for _, a := range m.agents {
		for _, th := range a.Threads {
			k := th.Th + "\x00" + minmax(a.Key, th.Peer)
			if seen[k] {
				continue
			}
			seen[k] = true
			theirs := th.Theirs
			if o, ok := byKey[th.Peer]; ok {
				for _, ot := range o.Threads {
					if ot.Th == th.Th && ot.Peer == a.Key {
						theirs = ot.Mine // the peer's own word beats hearsay
					}
				}
			}
			rows = append(rows, []string{
				lipgloss.NewStyle().Foreground(t.agentColor(a.Key)).Render(a.Name) + t.faintText.Render(" ⇄ ") +
					lipgloss.NewStyle().Foreground(t.agentColor(th.Peer)).Render(m.nameOf(th.Peer)),
				th.Subject,
				t.badge(th.Mine) + t.faintText.Render("  ⇄  ") + t.badge(theirs),
				ago(m.now, th.Updated),
			})
		}
	}
	if len(rows) > 0 {
		b.WriteString("\n" + t.accentText.Render("  Threads in flight") + "\n")
		tb := table.New().
			Headers("BETWEEN", "SUBJECT", "STATES (LEFT ⇄ RIGHT)", "UPDATED").
			Rows(rows...).
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(t.border)).
			Wrap(false).
			StyleFunc(func(row, col int) lipgloss.Style {
				s := lipgloss.NewStyle().Padding(0, 1)
				if row == table.HeaderRow {
					return s.Foreground(t.dim).Bold(true)
				}
				return s
			})
		for _, l := range strings.Split(tb.Render(), "\n") {
			b.WriteString("  " + l + "\n")
		}
	}
	if len(m.agents) > 0 && !m.anyPresence() {
		b.WriteString("\n" + t.faintText.Render("  Only this host's own peers are visible. Agents started with `holler up --presence`\n  publish what they are doing, and it reaches every connected host.") + "\n")
	}
	return b.String()
}

func minmax(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

func (m model) anyPresence() bool {
	for _, a := range m.agents {
		if a.Presence && !a.Self {
			return true
		}
	}
	return false
}

// kindLabel names an activity item's kind for the Activity table.
func kindLabel(it feedItem) string {
	if it.Kind == "remote" {
		if strings.HasPrefix(it.Text, "is now ") {
			return "state"
		}
		return "thread"
	}
	return it.Kind
}

// activityView is the whole activity log as a table, newest first.
func (m model) activityView() string {
	t := m.th
	items := m.visibleFeed()
	h := m.layout().body.h - 5
	start := len(items) - 1 - m.actOff
	var rows [][]string
	for i := start; i >= 0 && len(rows) < max(1, h); i-- {
		it := items[i]
		text := it.Text
		if it.Via != "" {
			text += " via " + m.nameOf(it.Via)
		}
		to := ""
		if it.To != "" {
			to = lipgloss.NewStyle().Foreground(t.agentColor(it.To)).Render(m.nameOf(it.To))
		}
		rows = append(rows, []string{
			it.At.Local().Format("15:04:05"),
			lipgloss.NewStyle().Foreground(t.agentColor(it.From)).Render(m.nameOf(it.From)),
			kindLabel(it),
			to,
			text,
			it.Subject,
		})
	}
	tb := table.New().
		Headers("TIME", "FROM", "KIND", "TO", "WHAT", "THREAD").
		Rows(rows...).
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(t.border)).
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case row == table.HeaderRow:
				return s.Foreground(t.dim).Bold(true)
			case col == 0:
				return s.Foreground(t.faint)
			case col == 2:
				return s.Foreground(t.accent)
			case col == 5:
				return s.Foreground(t.dim)
			}
			return s
		}).
		Width(m.w - 2)
	title := t.accentText.Render(" Activity") + t.faintText.Render(fmt.Sprintf("  %d events · ↑/↓ scroll · esc back", len(items)))
	if len(items) == 0 {
		return title + "\n\n" + t.faintText.Render("  waiting for activity…")
	}
	return title + "\n" + tb.Render()
}

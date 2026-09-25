package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/render"
	"github.com/hollerprotocol/holler/wire"
)

// preview is the live conversation of the selected thread, shown on the
// dashboard without opening it.
type preview struct {
	peer, th string
	events   []api.Event
	seqs     map[int64]bool
	loaded   bool
}

func (p *preview) add(ev api.Event) {
	if p.seqs == nil {
		p.seqs = map[int64]bool{}
	}
	if !p.seqs[ev.Seq] {
		p.seqs[ev.Seq] = true
		p.events = append(p.events, ev)
	}
}

func (m model) selectedThread() *threadRow {
	ts := m.visibleThreads()
	if len(ts) == 0 {
		return nil
	}
	t := ts[min(m.threadSel, len(ts)-1)]
	return &t
}

// syncPreview follows the selection: a local thread's conversation is
// fetched once and then kept current by the event stream.
func (m *model) syncPreview() tea.Cmd {
	t := m.selectedThread()
	if t == nil || !t.Local {
		m.preview = nil
		return nil
	}
	if p := m.preview; p != nil && p.th == t.Th && p.peer == t.LocalPeer {
		return nil
	}
	m.preview = &preview{peer: t.LocalPeer, th: t.Th}
	return m.fetchThread(t.LocalPeer, t.Th)
}

func (m model) previewPane(r rect) string {
	th := m.th
	inner := r.w - 4
	t := m.selectedThread()
	if t == nil {
		return m.box("Conversation", -1, []string{"", th.faintText.Render("  select a thread")}, r.w, r.h, false)
	}
	a := m.selectedAgent()
	if !t.Local {
		lines := []string{
			"",
			" " + th.bold.Render(t.Subject),
			"",
			" " + lipgloss.NewStyle().Foreground(th.agentColor(a.Key)).Render(a.Name) + "  " + th.badge(t.Mine),
			" " + lipgloss.NewStyle().Foreground(th.agentColor(t.Peer)).Render(t.PeerName) + "  " + th.badge(t.Theirs),
			"",
			th.faintText.Render(" updated " + ago(m.now, t.Updated) + " ago"),
			"",
			th.faintText.Render(" The conversation is private to these two agents."),
			th.faintText.Render(" Presence shares only its subject and states."),
		}
		return m.box("Thread · "+t.Subject, -1, lines, r.w, r.h, false)
	}
	p := m.preview
	if p == nil || !p.loaded {
		return m.box("Conversation · "+t.Subject, -1, []string{"", "  " + m.spin.View() + th.dimText.Render(" loading…")}, r.w, r.h, false)
	}
	var lines []string
	for _, ev := range p.events {
		switch ev.Type {
		case wire.TMsg:
			lines = append(lines, m.previewMsg(ev, inner)...)
		case wire.TState:
			var s wire.State
			json.Unmarshal(ev.Msg, &s)
			who := ev.Peer
			if ev.Dir == "out" {
				who = m.self
			}
			l := th.faintText.Render("── ") + lipgloss.NewStyle().Foreground(th.agentColor(who)).Render(m.nameOf(who)) +
				th.dimText.Render(" is ") + th.badge(s.State)
			if s.Note != "" {
				l += th.dimText.Render(" · " + s.Note)
			}
			lines = append(lines, l, "")
		case "blob":
			name := ""
			if v, ok := ev.Meta["name"]; ok && v != nil {
				name = fmt.Sprint(v)
			}
			lines = append(lines, th.faintText.Render("⬇ received ")+lipgloss.NewStyle().Foreground(th.info).Render(name), "")
		}
	}
	// Newest at the bottom, like a chat.
	if n := r.h - 2; len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return m.box("Conversation · "+t.Subject, -1, lines, r.w, r.h, false)
}

func (m model) previewMsg(ev api.Event, width int) []string {
	th := m.th
	var msg wire.Msg
	json.Unmarshal(ev.Msg, &msg)
	from := ev.Peer
	if ev.Dir == "out" {
		from = m.self
	}
	c := th.agentColor(from)
	head := lipgloss.NewStyle().Foreground(c).Bold(true).Render(m.nameOf(from)) + th.faintText.Render(" · "+ev.At.Local().Format("15:04:05"))
	if ev.Dir == "out" && ev.Acked {
		head += lipgloss.NewStyle().Foreground(th.good).Render(" ✓")
	}
	var md strings.Builder
	var files []string
	for _, p := range msg.Parts {
		switch p.K {
		case wire.PartText:
			md.WriteString(p.Text + "\n\n")
		case wire.PartCode:
			md.WriteString("```" + p.Lang + "\n" + strings.TrimRight(p.Text, "\n") + "\n```\n\n")
		case wire.PartData:
			md.WriteString("```json\n" + string(p.Data) + "\n```\n\n")
		case wire.PartBlob:
			f := lipgloss.NewStyle().Foreground(th.info).Render("📎 "+p.Name) + th.faintText.Render(" "+render.HumanSize(p.Size))
			if info, ok := ev.Blobs[p.Ref]; ok && info.Status != "complete" && ev.Dir != "out" && p.Size > 0 {
				f += " " + m.prog.ViewAs(float64(info.Received)/float64(p.Size))
			}
			files = append(files, f)
		}
	}
	bar := lipgloss.NewStyle().Foreground(c).Render("▎")
	out := []string{bar + head}
	if md.Len() > 0 {
		for _, l := range strings.Split(m.md.render("p"+ev.ID, md.String(), max(10, width-2), m.dark), "\n") {
			out = append(out, bar+" "+l)
		}
	}
	for _, f := range files {
		out = append(out, bar+" "+f)
	}
	return append(out, "")
}

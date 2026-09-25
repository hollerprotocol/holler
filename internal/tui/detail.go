package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/render"
	"github.com/hollerprotocol/holler/internal/store"
	"github.com/hollerprotocol/holler/wire"
)

// detailView is an open conversation: a title bar and a scrolling
// viewport of chat bubbles.
func (m model) detailView() string {
	t := m.th
	d := m.detail
	var th *store.Thread
	if m.snap != nil {
		for _, x := range m.snap.Threads {
			if x.Th == d.th && x.Peer == d.peer {
				th = x
			}
		}
	}
	title := t.accentText.Render(" " + d.th)
	if th != nil {
		title = " " + t.bold.Render(th.Subject) + t.faintText.Render("  "+d.th)
		states := t.dimText.Render("you ") + t.badge(th.MyState) + t.faintText.Render("   ") +
			t.dimText.Render(m.nameOf(d.peer)+" ") + t.badge(th.TheirState)
		title = m.spread(title, states+" ")
	}
	rule := lipgloss.NewStyle().Foreground(t.border).Render(strings.Repeat("─", max(0, m.w)))
	body := d.vp.View()
	switch {
	case d.err != nil:
		body = t.faintText.Render("  could not load the thread: " + d.err.Error())
	case !d.loaded:
		body = "  " + m.spin.View() + t.dimText.Render(" loading…")
	}
	return title + "\n" + rule + "\n" + body + "\n" + rule
}

// renderDetail lays out the conversation into the viewport.
func (m *model) renderDetail() {
	d := m.detail
	if d == nil || m.w == 0 {
		return
	}
	width := m.w
	bw := min(max(40, width*70/100), 110)
	var blocks []string
	for _, ev := range d.events {
		switch ev.Type {
		case wire.TMsg:
			blocks = append(blocks, m.bubble(ev, width, bw))
		case wire.TState:
			blocks = append(blocks, m.pill(ev, width))
		case "blob":
			blocks = append(blocks, lipgloss.PlaceHorizontal(width, lipgloss.Center,
				m.th.faintText.Render(fmt.Sprintf("⬇ received %v (%s)", ev.Meta["name"], render.HumanSize(int64(toFloat(ev.Meta["size"])))))))
		case wire.TErr:
			blocks = append(blocks, lipgloss.PlaceHorizontal(width, lipgloss.Center,
				lipgloss.NewStyle().Foreground(m.th.bad).Render(fmt.Sprintf("! %v: %v", ev.Meta["code"], ev.Meta["detail"]))))
		}
	}
	atBottom := d.vp.AtBottom() || d.vp.TotalLineCount() == 0
	d.vp.SetContent(strings.Join(blocks, "\n"))
	if atBottom {
		d.vp.GotoBottom()
	}
}

// bubble renders one message: ours on the right, theirs on the left,
// markdown through glamour, code and data as fenced blocks, files with a
// progress bar.
func (m *model) bubble(ev api.Event, width, bw int) string {
	t := m.th
	var msg wire.Msg
	json.Unmarshal(ev.Msg, &msg)
	from := ev.Peer
	if ev.Dir == "out" {
		from = m.self
	}
	color := t.agentColor(from)
	inner := bw - 4
	head := lipgloss.NewStyle().Foreground(color).Bold(true).Render(m.nameOf(from)) +
		t.faintText.Render(" · "+ev.At.Local().Format("15:04:05"))
	if ev.Dir == "out" && ev.Acked {
		head += lipgloss.NewStyle().Foreground(t.good).Render(" ✓")
	}
	var md strings.Builder
	for _, p := range msg.Parts {
		switch p.K {
		case wire.PartText:
			md.WriteString(p.Text + "\n\n")
		case wire.PartCode:
			md.WriteString("```" + p.Lang + "\n" + strings.TrimRight(p.Text, "\n") + "\n```\n\n")
		case wire.PartData:
			pretty, err := json.MarshalIndent(json.RawMessage(p.Data), "", "  ")
			if err != nil {
				pretty = p.Data
			}
			md.WriteString("```json\n" + string(pretty) + "\n```\n\n")
		}
	}
	body := ""
	if md.Len() > 0 {
		body = m.md.render(ev.ID, md.String(), inner, m.dark)
	}
	var files []string
	for _, p := range msg.Parts {
		if p.K != wire.PartBlob {
			continue
		}
		line := lipgloss.NewStyle().Foreground(t.info).Render("📎 "+p.Name) + t.faintText.Render(" "+render.HumanSize(p.Size))
		if info, ok := ev.Blobs[p.Ref]; ok {
			if info.Status == "complete" || ev.Dir == "out" {
				line += "  " + t.faintText.Render(shortPath(info.Path, inner-24))
			} else if p.Size > 0 {
				line += "  " + m.prog.ViewAs(float64(info.Received)/float64(p.Size))
			}
		}
		files = append(files, line)
	}
	content := head
	if body != "" {
		content += "\n" + body
	}
	if len(files) > 0 {
		content += "\n" + strings.Join(files, "\n")
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Render(content)
	if ev.Dir == "out" {
		return lipgloss.PlaceHorizontal(width, lipgloss.Right, box)
	}
	return box
}

// pill renders a state change, centered.
func (m *model) pill(ev api.Event, width int) string {
	t := m.th
	var s wire.State
	json.Unmarshal(ev.Msg, &s)
	who := ev.Peer
	if ev.Dir == "out" {
		who = m.self
	}
	text := lipgloss.NewStyle().Foreground(t.agentColor(who)).Render(m.nameOf(who)) + t.dimText.Render(" is ") + t.badge(s.State)
	if s.Note != "" {
		text += t.dimText.Render(" · " + s.Note)
	}
	line := t.faintText.Render("── ") + text + t.faintText.Render(" ──")
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, line)
}

func shortPath(p string, n int) string {
	if n < 10 || len(p) <= n {
		return p
	}
	return "…" + p[len(p)-n+1:]
}

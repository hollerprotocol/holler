package tui

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hollerprotocol/holler/wire"
)

// theme holds every color and style, built for the terminal's background
// (lipgloss v2 picks light or dark variants once the background is known).
type theme struct {
	dark bool

	fg, dim, faint, border, accent, accent2, good, warn, bad, info color.Color

	states  map[string]color.Color
	palette []color.Color

	text, bold, dimText, faintText, accentText lipgloss.Style
}

func newTheme(dark bool) theme {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	t := theme{dark: dark}
	t.fg = ld(c("#1E1B2E"), c("#E9E6F5"))
	t.dim = ld(c("#6B6785"), c("#A29FBA"))
	t.faint = ld(c("#A9A5C0"), c("#5B5873"))
	t.border = ld(c("#D5D1E8"), c("#3A3752"))
	t.accent = ld(c("#6B4EFF"), c("#A58BFF"))  // violet
	t.accent2 = ld(c("#E0347E"), c("#FF6FAE")) // pink
	t.good = ld(c("#1F9D63"), c("#5EE6A8"))
	t.warn = ld(c("#B7791F"), c("#FFCB6B"))
	t.bad = ld(c("#D23B3B"), c("#FF7575"))
	t.info = ld(c("#1B7FB5"), c("#6CCBFF"))
	t.states = map[string]color.Color{
		wire.StateOpen:    t.dim,
		wire.StateWorking: t.warn,
		wire.StateWaiting: t.accent2,
		wire.StateDone:    t.good,
		wire.StateFailed:  t.bad,
		wire.StateClosed:  t.faint,
	}
	t.palette = []color.Color{
		ld(c("#0E9F8E"), c("#4FE0C8")), // teal
		ld(c("#D6336C"), c("#FF7FAF")), // rose
		ld(c("#C77D00"), c("#FFC45E")), // amber
		ld(c("#2F6FDE"), c("#79A8FF")), // blue
		ld(c("#5C9E1A"), c("#A5E663")), // lime
		ld(c("#C2522B"), c("#FF9466")), // orange
		ld(c("#7E4DE0"), c("#C3A6FF")), // lilac
		ld(c("#118AB2"), c("#67D5F5")), // cyan
	}
	t.text = lipgloss.NewStyle().Foreground(t.fg)
	t.bold = t.text.Bold(true)
	t.dimText = lipgloss.NewStyle().Foreground(t.dim)
	t.faintText = lipgloss.NewStyle().Foreground(t.faint)
	t.accentText = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	return t
}

// agentColor gives each agent a stable color of its own.
func (t theme) agentColor(key string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(key))
	return t.palette[int(h.Sum32())%len(t.palette)]
}

func (t theme) stateColor(state string) color.Color {
	if c, ok := t.states[state]; ok {
		return c
	}
	return t.info // a state outside the recommended vocabulary
}

// badge renders a thread state as a colored dot and word.
func (t theme) badge(state string) string {
	if state == "" {
		state = wire.StateOpen
	}
	dot := "●"
	switch state {
	case wire.StateDone:
		dot = "✔"
	case wire.StateFailed:
		dot = "✘"
	case wire.StateClosed:
		dot = "◌"
	case wire.StateWaiting:
		dot = "◐"
	}
	return lipgloss.NewStyle().Foreground(t.stateColor(state)).Render(dot + " " + state)
}

// gradient paints text with a color blend, one color per rune.
func gradient(s string, stops ...color.Color) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	colors := lipgloss.Blend1D(max(len(runes), 2), stops...)
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// blend returns the color at position p (0..1) between two colors.
func blend(a, b color.Color, p float64) color.Color {
	steps := lipgloss.Blend1D(21, a, b)
	i := int(p*20 + 0.5)
	return steps[max(0, min(20, i))]
}

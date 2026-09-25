package tui

import (
	"math"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/harmonica"
)

type keyMap struct {
	Up, Down, Top, Bottom, Next, Prev, Open, Back key.Binding
	Dashboard, Network, Activity                  key.Binding
	Filter, Pause, Refresh, Help, Quit            key.Binding
}

func newKeyMap() keyMap {
	b := func(help, desc string, keys ...string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(help, desc))
	}
	return keyMap{
		Up:        b("↑/k", "up", "up", "k"),
		Down:      b("↓/j", "down", "down", "j"),
		Top:       b("g", "top", "g", "home"),
		Bottom:    b("G", "bottom", "G", "end"),
		Next:      b("tab", "next pane", "tab"),
		Prev:      b("shift+tab", "previous pane", "shift+tab"),
		Open:      b("enter", "open thread", "enter"),
		Back:      b("esc", "back", "esc"),
		Dashboard: b("1", "dashboard", "1"),
		Network:   b("2", "network", "2"),
		Activity:  b("3", "activity", "3"),
		Filter:    b("/", "filter", "/"),
		Pause:     b("p", "pause feed", "p"),
		Refresh:   b("r", "refresh", "r"),
		Help:      b("?", "more keys", "?"),
		Quit:      b("q", "quit", "q", "ctrl+c"),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Open, k.Dashboard, k.Network, k.Activity, k.Filter, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Next, k.Prev, k.Open, k.Back},
		{k.Dashboard, k.Network, k.Activity},
		{k.Filter, k.Pause, k.Refresh, k.Help, k.Quit},
	}
}

// spring animates one value with harmonica: selection bars sliding between
// rows, the activity pulse fading after an event.
type spring struct {
	pos, vel, target float64
	s                harmonica.Spring
}

func newSpring(freq, damping float64) spring {
	return spring{s: harmonica.NewSpring(harmonica.FPS(fps), freq, damping)}
}

const fps = 60

// step advances one frame and reports whether the spring is still moving.
func (sp *spring) step() bool {
	sp.pos, sp.vel = sp.s.Update(sp.pos, sp.vel, sp.target)
	if math.Abs(sp.pos-sp.target) < 0.002 && math.Abs(sp.vel) < 0.002 {
		sp.pos, sp.vel = sp.target, 0
		return false
	}
	return true
}

func (sp *spring) moving() bool {
	return sp.pos != sp.target || sp.vel != 0
}

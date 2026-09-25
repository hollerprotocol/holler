package tui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/render"
	"github.com/hollerprotocol/holler/internal/store"
	"github.com/hollerprotocol/holler/wire"
)

// Agent statuses as the dashboard shows them.
const (
	stYou          = "you"
	stConnected    = "connected"
	stOnline       = "online" // heard through presence, not connected here
	stReconnecting = "reconnecting"
	stStale        = "stale" // presence went quiet: asleep, or gone
	stBye          = "said bye"
	stOffline      = "offline"
)

var statusRank = map[string]int{stYou: 0, stConnected: 1, stOnline: 2, stReconnecting: 3, stStale: 4, stBye: 5, stOffline: 6}

// agentRow is one agent on the network.
type agentRow struct {
	Key, Short, Name, About, Version string
	Self                             bool
	Status                           string
	Direct                           bool   // a peer of this host
	Presence                         bool   // we have its signed presence
	Via                              string // relay its presence came through
	Hops                             int
	Seen                             time.Time
	Unread, Outbox                   int
	Threads                          []threadRow
	Peers                            []wire.PresencePeer
}

// threadRow is a thread seen from one agent's side.
type threadRow struct {
	Th, Subject    string
	Peer, PeerName string // the other party
	Mine, Theirs   string // this agent's state, and the other party's
	Unread         int
	Updated        time.Time
	Local          bool   // this host is a party, so the conversation can be opened
	LocalPeer      string // the other party as this host knows it
}

func (a *agentRow) count(state string) int {
	k := 0
	for _, t := range a.Threads {
		if t.Mine == state {
			k++
		}
	}
	return k
}

func (a *agentRow) active() int {
	k := 0
	for _, t := range a.Threads {
		if !store.Terminal(t.Mine) && !store.Terminal(t.Theirs) {
			k++
		}
	}
	return k
}

// buildAgents merges this host's peers with every agent heard of through
// presence, and returns them with a key-to-name map.
func buildAgents(s *Snapshot) ([]agentRow, map[string]string) {
	names := map[string]string{}
	if s == nil || s.Status == nil {
		return nil, names
	}
	st := s.Status
	self := st.Key
	for _, p := range st.Peers {
		names[p.Key] = p.Label()
	}
	if s.Net != nil {
		for _, a := range s.Net.Agents {
			if a.Name != "" {
				names[a.Origin] = a.Name
			}
			for _, pp := range a.Peers {
				if pp.Name != "" && names[pp.Key] == "" {
					names[pp.Key] = pp.Name
				}
			}
		}
	}
	names[self] = st.Name
	name := func(key string) string {
		if n := names[key]; n != "" {
			return n
		}
		return wire.ShortKey(key)
	}

	me := agentRow{Key: self, Short: st.Short, Name: st.Name, Version: st.Version, Self: true, Status: stYou,
		Presence: st.Presence, Seen: s.At, Unread: st.Unread, Outbox: st.Outbox}
	if s.Net != nil {
		me.About = s.Net.Self.About
	}
	for _, t := range s.Threads {
		me.Threads = append(me.Threads, threadRow{Th: t.Th, Subject: t.Subject, Peer: t.Peer, PeerName: name(t.Peer),
			Mine: t.MyState, Theirs: t.TheirState, Unread: t.Unread, Updated: t.Updated, Local: true, LocalPeer: t.Peer})
	}
	for _, p := range st.Peers {
		me.Peers = append(me.Peers, wire.PresencePeer{Key: p.Key, Name: p.Label(), Up: p.Connected})
	}

	byKey := map[string]*agentRow{}
	var order []string
	for _, p := range st.Peers {
		a := &agentRow{Key: p.Key, Short: p.Short, Name: p.Label(), About: p.About, Direct: true, Seen: p.LastSeen, Outbox: p.Outbox}
		switch {
		case p.Connected:
			a.Status = stConnected
		case p.Dialing:
			a.Status = stReconnecting
		case p.Parked:
			a.Status = stBye
		default:
			a.Status = stOffline
		}
		// Our threads with this peer, turned around to its point of view.
		for _, t := range s.Threads {
			if t.Peer == p.Key {
				a.Threads = append(a.Threads, threadRow{Th: t.Th, Subject: t.Subject, Peer: self, PeerName: st.Name,
					Mine: t.TheirState, Theirs: t.MyState, Updated: t.Updated, Local: true, LocalPeer: p.Key})
			}
		}
		a.Peers = []wire.PresencePeer{{Key: self, Name: st.Name, Up: p.Connected}}
		byKey[p.Key] = a
		order = append(order, p.Key)
	}
	if s.Net != nil {
		for _, v := range s.Net.Agents {
			a, ok := byKey[v.Origin]
			if !ok {
				a = &agentRow{Key: v.Origin, Short: v.Short, Name: v.Name, Status: stOffline}
				byKey[v.Origin] = a
				order = append(order, v.Origin)
			}
			a.Presence = true
			a.Version, a.Hops, a.Via = v.Version, v.Hops, v.Via
			if v.About != "" {
				a.About = v.About
			}
			if v.Name != "" {
				a.Name = v.Name
			}
			a.Seen = v.Time()
			a.Outbox, a.Unread = v.Outbox, v.Unread
			if a.Status != stConnected && a.Status != stReconnecting {
				if v.Status == api.AgentStale {
					a.Status = stStale
				} else {
					a.Status = stOnline
				}
			}
			// The agent's own account of its threads is the better one.
			a.Threads = nil
			for _, t := range v.Threads {
				upd, _ := wire.ParseTime(t.Updated)
				a.Threads = append(a.Threads, threadRow{Th: t.Th, Subject: t.Subject, Peer: t.Peer, PeerName: name(t.Peer),
					Mine: t.Mine, Theirs: t.Theirs, Unread: t.Unread, Updated: upd, Local: t.Peer == self, LocalPeer: v.Origin})
			}
			a.Peers = v.Peers
		}
	}
	rows := []agentRow{me}
	others := make([]agentRow, 0, len(order))
	for _, k := range order {
		others = append(others, *byKey[k])
	}
	slices.SortStableFunc(others, func(x, y agentRow) int {
		if d := statusRank[x.Status] - statusRank[y.Status]; d != 0 {
			return d
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	return append(rows, others...), names
}

// feedItem is one line of network activity.
type feedItem struct {
	At       time.Time
	Kind     string // msg, state, connect, disconnect, blob, grant, introduce, err, bye, refused, joined, left, remote
	From, To string // agent keys
	Via      string // for "joined": the relay it was heard through
	Th       string
	Subject  string
	Text     string
	Seq      int64 // local event sequence, 0 for items derived from presence
	Born     time.Time
}

func (f feedItem) haystack(names map[string]string) string {
	return strings.ToLower(strings.Join([]string{f.Kind, names[f.From], names[f.To], f.Th, f.Subject, f.Text}, " "))
}

// itemFromEvent turns a local log event into a feed line.
func itemFromEvent(ev api.Event, self string) feedItem {
	it := feedItem{At: ev.At, Kind: ev.Type, Th: ev.Th, Subject: ev.Subject, Seq: ev.Seq, From: ev.Peer, To: self}
	if ev.Dir == "out" {
		it.From, it.To = self, ev.Peer
	}
	meta := func(k string) string {
		if v, ok := ev.Meta[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	switch ev.Type {
	case wire.TMsg:
		it.Text = msgSnippet(ev)
	case wire.TState:
		var s wire.State
		json.Unmarshal(ev.Msg, &s)
		it.Text = s.State
		if s.Note != "" {
			it.Text += ": " + s.Note
		}
	case "connected":
		it.Kind, it.To, it.Text = "connect", self, "via "+shortAddr(meta("via"))
	case "disconnected":
		it.Kind, it.To, it.Text = "disconnect", self, meta("reason")
	case "blob":
		it.Text = fmt.Sprintf("%s (%s)", meta("name"), render.HumanSize(int64(toFloat(ev.Meta["size"]))))
	case "presence":
		it.Kind, it.To = meta("event"), ""
		if it.Kind == "joined" {
			it.Text = "joined the network"
			if via := meta("via"); via != "" && via != ev.Peer {
				it.Via = via
			}
		} else {
			it.Text = "went quiet"
		}
	case wire.TGrant:
		caps, _ := ev.Meta["caps"].([]any)
		var cs []string
		for _, c := range caps {
			cs = append(cs, fmt.Sprint(c))
		}
		it.Text = "[" + strings.Join(cs, ", ") + "]"
	case wire.TIntroduce:
		it.Text = "introduces " + meta("name")
	case wire.TErr:
		it.Text = meta("code") + ": " + meta("detail")
	case wire.TBye:
		it.Text = meta("reason")
	case "refused":
		it.Text = meta("reason")
	}
	return it
}

// plainMarkdown strips the emphasis markers that read as noise in a
// one-line snippet.
var plainMarkdown = strings.NewReplacer("**", "", "__", "", "`", "", "*", "", "# ", "")

func msgSnippet(ev api.Event) string {
	var parts []string
	for _, p := range ev.Parts() {
		switch p.K {
		case wire.PartText:
			parts = append(parts, plainMarkdown.Replace(strings.Join(strings.Fields(p.Text), " ")))
		case wire.PartCode:
			parts = append(parts, "[code "+p.Lang+"]")
		case wire.PartData:
			parts = append(parts, "[data]")
		case wire.PartBlob:
			parts = append(parts, "[file "+p.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func shortAddr(a string) string {
	if len(a) > 28 {
		return a[:18] + "…" + a[len(a)-6:]
	}
	return a
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}

// diffPresence finds what other agents did between two snapshots in
// conversations this host is not part of (its own come from the event
// log), so the feed covers the whole network.
func diffPresence(prev, cur []agentRow, self string, now time.Time) []feedItem {
	before := map[string]agentRow{}
	// Threads already known from either side: both parties report a new
	// thread, and it should be announced once.
	known := map[string]bool{}
	for _, a := range prev {
		before[a.Key] = a
		for _, t := range a.Threads {
			known[t.Th+"\x00"+minmax(a.Key, t.Peer)] = true
		}
	}
	var out []feedItem
	for _, a := range cur {
		old, seen := before[a.Key]
		if a.Self || !a.Presence || !seen || !old.Presence {
			continue
		}
		was := map[string]threadRow{}
		for _, t := range old.Threads {
			was[t.Th+"\x00"+t.Peer] = t
		}
		for _, t := range a.Threads {
			if t.Peer == self {
				continue
			}
			o, ok := was[t.Th+"\x00"+t.Peer]
			pair := t.Th + "\x00" + minmax(a.Key, t.Peer)
			switch {
			case !ok && known[pair]:
			case !ok:
				known[pair] = true
				out = append(out, feedItem{At: now, Kind: "remote", From: a.Key, To: t.Peer, Th: t.Th, Subject: t.Subject, Text: "opened a thread", Born: now})
			case o.Mine != t.Mine:
				out = append(out, feedItem{At: now, Kind: "remote", From: a.Key, To: t.Peer, Th: t.Th, Subject: t.Subject, Text: "is now " + t.Mine, Born: now})
			}
		}
	}
	return out
}

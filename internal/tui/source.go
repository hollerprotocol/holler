package tui

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/control"
	"github.com/hollerprotocol/holler/internal/store"
)

// Snapshot is everything the dashboard shows that is polled rather than
// streamed.
type Snapshot struct {
	Status  *api.Status
	Threads []*store.Thread // this host's threads
	Net     *api.Network    // presence: this agent and every agent heard of
	At      time.Time
}

// Source is where the dashboard gets its data. The real one talks to the
// local daemon over its control socket; tests use a fake.
type Source interface {
	Snapshot(ctx context.Context) (*Snapshot, error)
	// History returns recent events, oldest first.
	History(ctx context.Context, limit int) ([]api.Event, error)
	// Subscribe streams new events after seq until ctx ends or the stream
	// breaks.
	Subscribe(ctx context.Context, after int64, fn func(api.Event)) error
	// Thread returns one local thread's conversation.
	Thread(ctx context.Context, peer, th string) ([]api.Event, error)
}

// ControlSource reads from a daemon's control socket. It never starts a
// daemon and never marks anything read: watching is not reading.
type ControlSource struct {
	C *control.Client
}

// Snapshot implements Source.
func (s ControlSource) Snapshot(ctx context.Context) (*Snapshot, error) {
	snap := &Snapshot{At: time.Now()}
	var st api.Status
	if err := s.C.Call(ctx, "status", nil, &st); err != nil {
		return nil, err
	}
	snap.Status = &st
	if err := s.C.Call(ctx, "threads", api.PeerParams{}, &snap.Threads); err != nil {
		return nil, err
	}
	var net api.Network
	if err := s.C.Call(ctx, "presence", nil, &net); err != nil {
		// A daemon from before presence existed: show the local view.
		net = api.Network{Self: api.AgentView{Short: st.Short, Status: api.AgentSelf}}
		net.Self.Origin, net.Self.Name, net.Self.Version = st.Key, st.Name, st.Version
	}
	snap.Net = &net
	return snap, nil
}

// History implements Source.
func (s ControlSource) History(ctx context.Context, limit int) ([]api.Event, error) {
	var res api.ReadResult
	err := s.C.Call(ctx, "read", api.ReadParams{Limit: limit}, &res)
	return res.Events, err
}

// Subscribe implements Source.
func (s ControlSource) Subscribe(ctx context.Context, after int64, fn func(api.Event)) error {
	return s.C.Stream(ctx, "subscribe", api.SubscribeParams{Since: &after}, func(raw json.RawMessage) error {
		var ev api.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return err
		}
		fn(ev)
		return nil
	})
}

// Thread implements Source.
func (s ControlSource) Thread(ctx context.Context, peer, th string) ([]api.Event, error) {
	var res api.ReadResult
	err := s.C.Call(ctx, "read", api.ReadParams{Peer: peer, Th: th, Limit: 500}, &res)
	return res.Events, err
}

// stream keeps a subscription alive across daemon restarts, delivering
// events on ch and resuming after the last one seen.
func stream(ctx context.Context, src Source, after int64, ch chan<- api.Event) {
	for ctx.Err() == nil {
		src.Subscribe(ctx, after, func(ev api.Event) {
			if ev.Seq > after {
				after = ev.Seq
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

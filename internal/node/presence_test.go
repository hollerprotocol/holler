package node

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hollerprotocol/holler/internal/store"
	"github.com/hollerprotocol/holler/wire"
)

func presenceOf(t *testing.T, n *Node, origin string) *store.PresenceRow {
	t.Helper()
	rows, err := n.Store().PresenceSince(time.Now().Add(-presenceMaxAge))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Origin == origin {
			return r
		}
	}
	return nil
}

// TestPresenceGossipChain: C shares its presence; B relays it without
// sharing its own; A, two hops away, sees C through B.
func TestPresenceGossipChain(t *testing.T) {
	share := func(c *Config) { c.Presence = true }
	a := testNode(t, "a")
	b := testNode(t, "b")
	c := testNode(t, "c", share)
	connect(t, c, b)
	res, err := c.Send(SendRequest{Peer: b.Key(), Subject: "Run the suite", Parts: text(t, "please")})
	if err != nil {
		t.Fatal(err)
	}
	c.SetState(b.Key(), res.Th, wire.StateWorking, "cloning")
	connect(t, a, b)

	waitFor(t, a, "c's presence at a", func() bool { return presenceOf(t, a, c.Key()) != nil })
	row := presenceOf(t, a, c.Key())
	if row.Via != b.Key() || row.Hops != 1 || row.Name != "c" {
		t.Fatalf("relayed presence: via %s hops %d name %q", wire.ShortKey(row.Via), row.Hops, row.Name)
	}
	p, err := wire.ParsePresence(row.Doc)
	if err != nil {
		t.Fatalf("stored document no longer verifies: %v", err)
	}
	var th *wire.PresenceThread
	for i := range p.Threads {
		if p.Threads[i].Th == res.Th {
			th = &p.Threads[i]
		}
	}
	if th == nil || th.Subject != "Run the suite" || th.Mine != wire.StateWorking || th.Peer != b.Key() {
		t.Fatalf("thread in presence: %+v", p.Threads)
	}
	// The relay stored it too, and never published itself: b opted out.
	if presenceOf(t, b, c.Key()) == nil {
		t.Fatal("relay did not store c's presence")
	}
	time.Sleep(3 * time.Second)
	if presenceOf(t, a, b.Key()) != nil || presenceOf(t, c, b.Key()) != nil {
		t.Fatal("an opted-out node published its presence")
	}
	// A new agent is announced once in the event log.
	recs, _ := a.Store().Query(store.Filter{Peer: c.Key(), Dirs: []string{"sys"}, Types: []string{"presence"}})
	if len(recs) != 1 || recs[0].Meta["event"] != "joined" {
		t.Fatalf("joined events: %+v", recs)
	}
}

// TestPresenceRejectsForgedAndStale feeds documents through a raw peer.
func TestPresenceRejectsForgedAndStale(t *testing.T) {
	n := testNode(t, "n")
	p := dialRaw(t, n)
	p.handshake()
	waitFor(t, n, "connected", func() bool { return n.Connected(p.key) })

	send := func(doc json.RawMessage) {
		line, _ := wire.Encode(wire.PresenceMsg{Envelope: wire.Envelope{T: wire.TPresence, ID: wire.NewIDGen().New(), TS: wire.Now()}, Doc: doc})
		p.send(string(line))
	}
	sign := func(seq int64, ts time.Time, name string) json.RawMessage {
		raw, err := wire.SignPresence(p.priv, &wire.Presence{Name: name, Seq: seq, TS: wire.FormatTime(ts)})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	// Forged: a valid document with its name changed after signing.
	var obj map[string]any
	json.Unmarshal(sign(1, time.Now(), "honest"), &obj)
	obj["name"] = "forged"
	forged, _ := json.Marshal(obj)
	send(forged)
	// Stale: correctly signed but eleven minutes old.
	send(sign(2, time.Now().Add(-11*time.Minute), "stale"))
	// Then a good one, so we know the others had their chance.
	send(sign(3, time.Now(), "honest"))
	waitFor(t, n, "honest presence", func() bool { return presenceOf(t, n, p.key) != nil })
	if r := presenceOf(t, n, p.key); r.Name != "honest" || r.Seq != 3 {
		t.Fatalf("stored %q seq %d", r.Name, r.Seq)
	}
	// An older sequence number never replaces a newer one.
	send(sign(2, time.Now(), "older"))
	p.send(`{"t":"ping","id":"p1","ts":"x"}`)
	for {
		if env, _ := p.read(); env.T == wire.TPong {
			break
		}
	}
	if r := presenceOf(t, n, p.key); r.Name != "honest" {
		t.Fatalf("older document replaced a newer one: %q", r.Name)
	}
	// The connection survives all of it: presence problems are never fatal.
	if !n.Connected(p.key) {
		t.Fatal("connection closed over a bad presence document")
	}
}

func TestPresenceExpiry(t *testing.T) {
	n := testNode(t, "n")
	old := time.Now().Add(-presenceMaxAge - time.Minute)
	for i, name := range []string{"sleepy", "gone"} {
		origin := fmt.Sprintf("ed25519:%043d", i)
		if _, _, err := n.Store().PutPresence(&store.PresenceRow{Origin: origin, Seq: 1, Name: name, TS: old, Received: old, Doc: []byte("{}")}); err != nil {
			t.Fatal(err)
		}
	}
	fresh := "ed25519:" + fmt.Sprintf("%043d", 9)
	n.Store().PutPresence(&store.PresenceRow{Origin: fresh, Seq: 1, Name: "awake", TS: time.Now(), Received: time.Now(), Doc: []byte("{}")})
	n.expirePresence()
	rows, _ := n.Store().PresenceSince(time.Time{})
	if len(rows) != 1 || rows[0].Name != "awake" {
		t.Fatalf("after expiry: %+v", rows)
	}
	recs, _ := n.Store().Query(store.Filter{Dirs: []string{"sys"}, Types: []string{"presence"}})
	if len(recs) != 2 || recs[0].Meta["event"] != "left" {
		t.Fatalf("left events: %+v", recs)
	}
}

// TestPresenceDedupStopsGossip: a document already stored is not forwarded
// again, which is what stops it circulating in a cycle.
func TestPresenceDedupStopsGossip(t *testing.T) {
	share := func(c *Config) { c.Presence = true }
	a := testNode(t, "a", share)
	b := testNode(t, "b")
	c := testNode(t, "c")
	connect(t, a, b)
	connect(t, b, c)
	connect(t, c, a) // a triangle
	waitFor(t, b, "a's presence at b", func() bool { return presenceOf(t, b, a.Key()) != nil })
	waitFor(t, c, "a's presence at c", func() bool { return presenceOf(t, c, a.Key()) != nil })
	seqB, seqC := presenceOf(t, b, a.Key()).Seq, presenceOf(t, c, a.Key()).Seq
	time.Sleep(2 * time.Second)
	// Nothing new from a: the copies b and c forwarded to each other were
	// dropped as duplicates, so the sequence numbers are unchanged.
	if presenceOf(t, b, a.Key()).Seq != seqB || presenceOf(t, c, a.Key()).Seq != seqC {
		t.Fatal("presence kept changing without a new document from its origin")
	}
	// a never stores its own document, even when it comes back around.
	if presenceOf(t, a, a.Key()) != nil {
		t.Fatal("origin stored its own presence")
	}
}

// Package node is the holler protocol engine: it owns the identity key,
// the durable store, the listeners and every connection, and implements the
// handshake, resume, acks, blobs, grants and reconnection of SPEC.md.
package node

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hollerprotocol/holler/internal/store"
	"github.com/hollerprotocol/holler/internal/transport"
	"github.com/hollerprotocol/holler/wire"
)

// Config configures a Node. Zero values get the defaults from the spec.
type Config struct {
	// Home is the state directory (~/.holler by default in the CLI).
	Home string

	// Name and About are sent in hello.
	Name  string
	About string

	// Listen lists addresses to accept connections on: "tailcat",
	// "tcp:host:port" or "unix:/path".
	Listen []string

	// Advertise is the address sent in hello.addr so the peer can dial us
	// back. Empty means the tailcat address if there is one, otherwise the
	// first listen address. "none" disables it.
	Advertise string

	PingInterval     time.Duration // idle time before a ping; default 30s
	HandshakeTimeout time.Duration // default 30s
	BlobLimit        int64         // largest blob accepted or sent; default 50 MiB
	OutboxTTL        time.Duration // outbox retention; default 7 days
	MaxBackoff       time.Duration // reconnect backoff cap; default 60s
	DialGrace        time.Duration // delay before dialing a hello-learned address; default 15s

	Policy Policy

	// Logf receives the daemon's log. Nil discards it.
	Logf func(format string, args ...any)
	// TailcatLogf receives tailcat's (verbose) diagnostics. Nil discards them.
	TailcatLogf func(format string, args ...any)
	// Trace logs every line sent and received.
	Trace bool
	// AllowPlaintext permits plain TCP to public addresses (see
	// transport.Transport.AllowPublicTCP).
	AllowPlaintext bool
}

func (c *Config) setDefaults() {
	if c.PingInterval == 0 {
		c.PingInterval = 30 * time.Second
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = 30 * time.Second
	}
	if c.BlobLimit == 0 {
		c.BlobLimit = 50 << 20
	}
	if c.OutboxTTL == 0 {
		c.OutboxTTL = 7 * 24 * time.Hour
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = 60 * time.Second
	}
	if c.DialGrace == 0 {
		c.DialGrace = 15 * time.Second
	}
	if c.Logf == nil {
		c.Logf = func(string, ...any) {}
	}
	if c.Policy.Accept == "" {
		c.Policy.Accept = AcceptAny
	}
}

// Node is a running holler peer.
type Node struct {
	cfg  Config
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	key  string
	st   *store.Store
	ids  *wire.IDGen
	tr   *transport.Transport

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu         sync.Mutex
	peers      map[string]*peerState
	listeners  []*listener
	tailcat    *transport.TailcatListener
	tailcatErr string
	closed     bool

	cmu      sync.Mutex
	changeCh chan struct{}
}

type listener struct {
	addr string
	ln   net.Listener
}

// peerState is the in-memory state for one remote key.
type peerState struct {
	key    string
	conn   *Conn     // the active, authenticated connection
	since  time.Time // when conn became active
	dialer *dialer   // running reconnect loop, if any
}

// Open loads (or creates) the identity and store under cfg.Home. Call Start
// to begin listening and reconnecting.
func Open(cfg Config) (*Node, error) {
	cfg.setDefaults()
	if cfg.Home == "" {
		return nil, errors.New("node: Home not set")
	}
	if err := os.MkdirAll(cfg.Home, 0o700); err != nil {
		return nil, err
	}
	priv, err := loadIdentity(filepath.Join(cfg.Home, "identity.json"))
	if err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(cfg.Home, "holler.db"))
	if err != nil {
		return nil, err
	}
	n := &Node{
		cfg:      cfg,
		priv:     priv,
		pub:      priv.Public().(ed25519.PublicKey),
		st:       st,
		ids:      wire.NewIDGen(),
		peers:    map[string]*peerState{},
		changeCh: make(chan struct{}),
	}
	n.key = wire.FormatKey(n.pub)
	if last, err := st.MaxOwnID(); err == nil && last != "" {
		n.ids.Observe(last)
	}
	n.tr = &transport.Transport{AllowPublicTCP: cfg.AllowPlaintext}
	if cfg.TailcatLogf != nil {
		n.tr.Logf = cfg.TailcatLogf
	}
	// Name and about persist once set, so a daemon started on demand by
	// any command keeps the identity its user gave it.
	for _, f := range []struct {
		key string
		v   *string
	}{{"name", &n.cfg.Name}, {"about", &n.cfg.About}} {
		if *f.v != "" {
			st.SetKV(f.key, *f.v)
		} else if v, ok, _ := st.GetKV(f.key); ok {
			*f.v = v
		}
	}
	if n.cfg.Name == "" {
		host, _ := os.Hostname()
		n.cfg.Name = "holler@" + host
	}
	n.ctx, n.cancel = context.WithCancel(context.Background())
	return n, nil
}

// Key returns this peer's encoded public key.
func (n *Node) Key() string { return n.key }

// Name returns this peer's name.
func (n *Node) Name() string { return n.cfg.Name }

// Store exposes the store for read-only queries by the control server.
func (n *Node) Store() *store.Store { return n.st }

// Home returns the state directory.
func (n *Node) Home() string { return n.cfg.Home }

func (n *Node) logf(format string, args ...any) {
	n.cfg.Logf(format, args...)
}

// Start opens the listeners and starts reconnecting to peers that have
// unfinished business. A tailcat listener starts in the background, since
// picking a DERP region takes a few seconds.
func (n *Node) Start() error {
	for _, l := range n.cfg.Listen {
		if l == "tailcat" {
			n.wg.Add(1)
			go n.startTailcat()
			continue
		}
		a, err := transport.Parse(l)
		if err != nil {
			return err
		}
		ln, err := transport.Listen(a)
		if err != nil {
			return fmt.Errorf("listen %s: %w", l, err)
		}
		addr := a
		if a.Kind == transport.KindTCP {
			addr.Target = ln.Addr().String()
		}
		n.addListener(&listener{addr: addr.String(), ln: ln})
	}
	n.wg.Add(1)
	go n.maintain()
	return nil
}

func (n *Node) addListener(l *listener) {
	n.mu.Lock()
	n.listeners = append(n.listeners, l)
	n.mu.Unlock()
	n.logf("listening on %s", l.addr)
	n.wg.Add(1)
	go n.acceptLoop(l)
	n.bump()
}

func (n *Node) startTailcat() {
	defer n.wg.Done()
	backoff := 5 * time.Second
	for {
		ctx, cancel := context.WithTimeout(n.ctx, 60*time.Second)
		tl, err := transport.ListenTailcat(ctx, filepath.Join(n.cfg.Home, "tailcat.json"), n.tr.Logf)
		cancel()
		if err == nil {
			n.mu.Lock()
			if n.closed {
				n.mu.Unlock()
				tl.Close()
				return
			}
			n.tailcat = tl
			n.tailcatErr = ""
			n.mu.Unlock()
			n.addListener(&listener{addr: "tailcat:" + tl.Address(), ln: tl})
			return
		}
		n.mu.Lock()
		n.tailcatErr = err.Error()
		n.mu.Unlock()
		n.logf("tailcat listener: %v (retrying in %v)", err, backoff)
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Minute)
	}
}

func (n *Node) acceptLoop(l *listener) {
	defer n.wg.Done()
	for {
		raw, err := l.ln.Accept()
		if err != nil {
			if n.ctx.Err() == nil {
				n.logf("accept on %s: %v", l.addr, err)
			}
			return
		}
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			n.serveConn(raw, false, l.addr, nil)
		}()
	}
}

// Close stops everything. Connections are dropped without bye: the state
// is on disk and peers will resume.
func (n *Node) Close() error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	var conns []*Conn
	for _, p := range n.peers {
		if p.conn != nil {
			conns = append(conns, p.conn)
		}
	}
	ls := n.listeners
	n.mu.Unlock()
	n.cancel()
	for _, l := range ls {
		l.ln.Close()
	}
	for _, c := range conns {
		c.Close("shutdown")
	}
	n.wg.Wait()
	n.tr.Close()
	return n.st.Close()
}

// Addresses returns the addresses this node listens on, tailcat first.
func (n *Node) Addresses() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, l := range n.listeners {
		out = append(out, l.addr)
	}
	slices.SortStableFunc(out, func(a, b string) int {
		ta, tb := strings.HasPrefix(a, "tailcat:"), strings.HasPrefix(b, "tailcat:")
		switch {
		case ta && !tb:
			return -1
		case tb && !ta:
			return 1
		}
		return 0
	})
	return out
}

// TailcatAddress returns the bare tailcat address, if listening on tailcat.
func (n *Node) TailcatAddress() (addr, errMsg string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.tailcat != nil {
		return n.tailcat.Address(), ""
	}
	return "", n.tailcatErr
}

// advertised is the address sent in hello.addr.
func (n *Node) advertised() string {
	switch n.cfg.Advertise {
	case "none":
		return ""
	case "":
		addrs := n.Addresses()
		if len(addrs) > 0 {
			return addrs[0]
		}
		return ""
	}
	return n.cfg.Advertise
}

// --- change notification ---

// Changed returns a channel that is closed at the next state change: a log
// record, an ack, a connection coming or going. Waiters re-check their
// condition and call Changed again.
func (n *Node) Changed() <-chan struct{} {
	n.cmu.Lock()
	defer n.cmu.Unlock()
	return n.changeCh
}

func (n *Node) bump() {
	n.cmu.Lock()
	close(n.changeCh)
	n.changeCh = make(chan struct{})
	n.cmu.Unlock()
}

// --- peers ---

func (n *Node) peer(key string) *peerState {
	p := n.peers[key]
	if p == nil {
		p = &peerState{key: key}
		n.peers[key] = p
	}
	return p
}

// Connected reports whether key has an active connection.
func (n *Node) Connected(key string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.peers[key]
	return p != nil && p.conn != nil
}

// ConnInfo describes a live connection.
type ConnInfo struct {
	Outbound bool      `json:"outbound"`
	Via      string    `json:"via"`
	Since    time.Time `json:"since"`
}

// Conn returns information about key's active connection, if any.
func (n *Node) ConnInfo(key string) *ConnInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.peers[key]
	if p == nil || p.conn == nil {
		return nil
	}
	return &ConnInfo{Outbound: p.conn.outbound, Via: p.conn.via, Since: p.since}
}

// Dialing reports whether a reconnect loop is running for key, and its last
// error.
func (n *Node) Dialing(key string) (bool, string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.peers[key]
	if p == nil || p.dialer == nil {
		return false, ""
	}
	return true, p.dialer.lastErr()
}

// activate makes c the active connection for its peer. When a connection
// already exists the newer one normally wins, since the older one is most
// likely half dead (the peer slept and came back). If both came up within a
// few seconds of each other the two sides dialed simultaneously; then both
// keep the connection dialed by the smaller key, so they agree without
// talking about it.
//
// The old connection's reader is stopped before c becomes active, so there
// is never more than one connection feeding a peer's state. That matters
// because dedup and resume both assume one ordered stream per peer.
func (n *Node) activate(c *Conn) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return errors.New("shutting down")
	}
	p := n.peer(c.peerKey)
	old := p.conn
	if old != nil && time.Since(p.since) < 5*time.Second && old.dialerKey() < c.dialerKey() {
		n.mu.Unlock()
		return errors.New("duplicate connection (simultaneous open, keeping the other one)")
	}
	p.conn = nil
	n.mu.Unlock()

	if old != nil {
		old.Close("replaced by a newer connection")
		<-old.readerDone
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if p.conn != nil {
		// Another connection won the race while we waited.
		return errors.New("duplicate connection")
	}
	p.conn = c
	p.since = time.Now()
	return nil
}

// deactivate is called when c ends.
func (n *Node) deactivate(c *Conn) {
	n.mu.Lock()
	p := n.peers[c.peerKey]
	wasActive := p != nil && p.conn == c
	if wasActive {
		p.conn = nil
	}
	closed := n.closed
	n.mu.Unlock()
	if !wasActive || closed {
		return
	}
	n.sysEvent(c.peerKey, "disconnected", map[string]any{"reason": c.closeReason(), "via": c.via})
	n.ensureDialer(c.peerKey, false)
}

// kick wakes the peer's connection writer, or its reconnect loop.
func (n *Node) kick(key string) {
	n.mu.Lock()
	p := n.peers[key]
	var c *Conn
	var d *dialer
	if p != nil {
		c, d = p.conn, p.dialer
	}
	n.mu.Unlock()
	if c != nil {
		c.kickPump()
	} else if d != nil {
		d.wake()
	} else {
		n.ensureDialer(key, true)
	}
}

// sysEvent records a local event in the log (dir "sys") and wakes waiters.
func (n *Node) sysEvent(peer, t string, detail map[string]any) {
	line, _ := wire.Encode(detail)
	rec := &store.Record{Peer: peer, Dir: "sys", ID: n.ids.New(), T: t, Line: line, At: time.Now(), Meta: detail}
	if th, ok := detail["th"].(string); ok {
		rec.Th = th
	}
	if err := n.st.Tx(func(q store.Q) error {
		_, err := store.Append(q, rec)
		return err
	}); err != nil {
		n.logf("log %s event: %v", t, err)
	}
	n.bump()
}

// maintain expires old outbox entries and makes sure every peer with
// unfinished business has a reconnect loop.
func (n *Node) maintain() {
	defer n.wg.Done()
	n.reconnectAll()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-t.C:
		}
		if k, err := n.st.ExpireOutbox(time.Now().Add(-n.cfg.OutboxTTL)); err != nil {
			n.logf("expire outbox: %v", err)
		} else if k > 0 {
			n.logf("expired %d outbox entries older than %v", k, n.cfg.OutboxTTL)
		}
		n.reconnectAll()
	}
}

func (n *Node) reconnectAll() {
	peers, err := n.st.Peers()
	if err != nil {
		n.logf("list peers: %v", err)
		return
	}
	for _, p := range peers {
		n.ensureDialer(p.Key, false)
	}
}

// wantConnection is the rule from section 4.3: keep trying for as long as
// there are unacked messages or open threads, unless the peer is parked
// (a bye was exchanged and nothing new has been queued since).
func (n *Node) wantConnection(key string) bool {
	p, err := store.GetPeer(n.st.DB(), key)
	if err != nil || p == nil || p.Parked {
		return false
	}
	if k, _ := n.st.OutboxCount(key); k > 0 {
		return true
	}
	open, _ := n.st.OpenThreads(key, time.Now().Add(-n.cfg.OutboxTTL))
	return open > 0
}

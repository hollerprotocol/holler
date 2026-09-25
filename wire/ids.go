package wire

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"sync"
	"time"
)

// crockford is the ULID base32 alphabet.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// IDGen generates ULIDs that strictly increase, even within one millisecond
// and even if the wall clock steps backwards (a sandbox restored from a
// snapshot can do that). Resume compares ids as strings, so monotonicity is
// what keeps "ids greater than the seen id" meaningful.
type IDGen struct {
	mu   sync.Mutex
	last [16]byte
	now  func() time.Time
}

// NewIDGen returns a generator. Call Observe with the newest id persisted by
// a previous run so that ids keep increasing across restarts.
func NewIDGen() *IDGen {
	return &IDGen{now: time.Now}
}

// New returns the next id.
func (g *IDGen) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var u [16]byte
	ms := uint64(g.now().UnixMilli())
	putMillis(&u, ms)
	rand.Read(u[6:])
	if lastMs := millis(g.last); ms <= lastMs {
		// Same or earlier millisecond: continue from the last id.
		u = g.last
		increment(&u)
	}
	g.last = u
	return encodeULID(u)
}

// Observe records an id produced earlier (possibly by a previous process) so
// that New never returns anything smaller.
func (g *IDGen) Observe(id string) {
	u, ok := decodeULID(id)
	if !ok {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if compare(u, g.last) > 0 {
		g.last = u
	}
}

// IDTime returns the timestamp embedded in a ULID, or false if id is not one.
func IDTime(id string) (time.Time, bool) {
	u, ok := decodeULID(id)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(millis(u))), true
}

func putMillis(u *[16]byte, ms uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], ms)
	copy(u[:6], b[2:])
}

func millis(u [16]byte) uint64 {
	var b [8]byte
	copy(b[2:], u[:6])
	return binary.BigEndian.Uint64(b[:])
}

func increment(u *[16]byte) {
	for i := 15; i >= 0; i-- {
		u[i]++
		if u[i] != 0 {
			return
		}
	}
}

func compare(a, b [16]byte) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// encodeULID writes 128 bits as 26 Crockford base32 characters (the first
// character carries only 3 bits).
func encodeULID(u [16]byte) string {
	var out [26]byte
	// Treat u as a 130-bit number with two leading zero bits.
	var acc uint64
	bits := 2 // leading pad bits
	idx := 0
	for _, b := range u {
		acc = acc<<8 | uint64(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[idx] = crockford[(acc>>uint(bits))&31]
			idx++
		}
	}
	return string(out[:])
}

func decodeULID(s string) ([16]byte, bool) {
	var u [16]byte
	if len(s) != 26 {
		return u, false
	}
	s = strings.ToUpper(s)
	var acc uint64
	bits := 0
	idx := 0
	for i := 0; i < 26; i++ {
		v := strings.IndexByte(crockford, s[i])
		if v < 0 {
			return u, false
		}
		if i == 0 && v > 7 {
			return u, false // overflow: first char carries 3 bits
		}
		acc = acc<<5 | uint64(v)
		bits += 5
		if i == 0 {
			bits = 3
		}
		for bits >= 8 {
			bits -= 8
			u[idx] = byte(acc >> uint(bits))
			idx++
		}
	}
	return u, idx == 16
}

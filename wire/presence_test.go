package wire

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPresenceSignAndVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := &Presence{
		Name:    "claude-code@worker",
		Version: "0.1.0",
		Seq:     7,
		TS:      FormatTime(time.Now()),
		Peers:   []PresencePeer{{Key: FormatKey(pub), Name: "boss", Up: true}},
		Threads: []PresenceThread{{Th: "thr_1", Peer: FormatKey(pub), Subject: "Fix <the> tests & ship", Mine: "working", Theirs: "open"}},
		Outbox:  2,
	}
	raw, err := SignPresence(priv, p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePresence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Origin != FormatKey(pub) || got.Seq != 7 || got.Threads[0].Mine != "working" || got.Outbox != 2 {
		t.Fatalf("round trip: %+v", got)
	}
	// Relays forward the document unchanged; re-encoding with other key
	// order still verifies, because the signature covers canonical JSON.
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	pretty, _ := json.MarshalIndent(obj, "", "  ")
	if _, err := ParsePresence(pretty); err != nil {
		t.Fatalf("re-encoded: %v", err)
	}
	// Any change breaks it.
	obj["name"] = "someone-else"
	forged, _ := json.Marshal(obj)
	if _, err := ParsePresence(forged); !errors.Is(err, ErrPresenceInvalid) {
		t.Fatalf("forged document: %v", err)
	}
	// A document signed by one key cannot claim another origin.
	_, other, _ := ed25519.GenerateKey(nil)
	raw2, _ := SignPresence(other, &Presence{Seq: 1, TS: FormatTime(time.Now())})
	var obj2 map[string]any
	json.Unmarshal(raw2, &obj2)
	obj2["origin"] = FormatKey(pub)
	claimed, _ := json.Marshal(obj2)
	if _, err := ParsePresence(claimed); !errors.Is(err, ErrPresenceInvalid) {
		t.Fatalf("stolen origin accepted: %v", err)
	}
}

func TestPresenceSizeCap(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	p := &Presence{Seq: 1, TS: FormatTime(time.Now()), About: strings.Repeat("x", MaxPresenceBytes)}
	if _, err := SignPresence(priv, p); err == nil {
		t.Fatal("oversized presence signed")
	}
	if _, err := ParsePresence([]byte(`{"origin":"` + strings.Repeat("y", MaxPresenceBytes) + `"}`)); !errors.Is(err, ErrPresenceInvalid) {
		t.Fatalf("oversized presence parsed: %v", err)
	}
}

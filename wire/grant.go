package wire

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Grant is a signed statement that Sub may use Caps until Exp, honored by
// Iss and by whoever trusts Iss (section 10.2). Raw holds the exact object
// as issued or received; it is what travels on the wire, so fields this
// implementation does not know about survive and stay covered by the
// signature.
type Grant struct {
	Iss   string
	Sub   string
	Caps  []string
	Exp   time.Time
	Nonce string
	Sig   string

	// Aud is an extension: when set, only the peer with this key should
	// honor the grant. Introductions use it so that a grant meant for the
	// introduced peer does not also give the recipient powers over the
	// introducer. Peers that do not know the field ignore it, which can
	// only make them honor less, never more, than the spec already says.
	Aud string

	Raw json.RawMessage
}

// Has reports whether the grant names capability c.
func (g *Grant) Has(c string) bool {
	return slices.Contains(g.Caps, c)
}

// Hash identifies a grant by the SHA-256 of its canonical form, signature
// included.
func (g *Grant) Hash() string {
	c, err := Canonical(g.Raw)
	if err != nil {
		c = g.Raw
	}
	sum := sha256.Sum256(c)
	return hex.EncodeToString(sum[:16])
}

// MintGrant issues a grant from priv to sub. aud, if non-empty, restricts
// which peer should honor it (see Grant.Aud).
func MintGrant(priv ed25519.PrivateKey, sub string, caps []string, exp time.Time, aud string) (*Grant, error) {
	if _, err := ParseKey(sub); err != nil {
		return nil, fmt.Errorf("grant sub: %w", err)
	}
	if caps == nil {
		caps = []string{}
	}
	body := map[string]any{
		"iss":   FormatKey(priv.Public().(ed25519.PublicKey)),
		"sub":   sub,
		"caps":  caps,
		"exp":   exp.UTC().Format(time.RFC3339),
		"nonce": Nonce(16),
	}
	if aud != "" {
		if _, err := ParseKey(aud); err != nil {
			return nil, fmt.Errorf("grant aud: %w", err)
		}
		body["aud"] = aud
	}
	canon, err := CanonicalValue(body)
	if err != nil {
		return nil, err
	}
	body["sig"] = EncodeB64(ed25519.Sign(priv, canon))
	raw, err := CanonicalValue(body)
	if err != nil {
		return nil, err
	}
	return ParseGrant(raw, time.Time{})
}

// Errors from ParseGrant.
var (
	ErrGrantExpired = errors.New("grant expired")
	ErrGrantInvalid = errors.New("invalid grant")
)

// ParseGrant decodes and verifies a grant: the signature must be iss's over
// the canonical JSON of the object without sig. If now is non-zero the grant
// must not have expired at that time.
func ParseGrant(raw []byte, now time.Time) (*Grant, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%w: not a JSON object", ErrGrantInvalid)
	}
	str := func(k string) string {
		s, _ := obj[k].(string)
		return s
	}
	g := &Grant{
		Iss:   str("iss"),
		Sub:   str("sub"),
		Nonce: str("nonce"),
		Sig:   str("sig"),
		Aud:   str("aud"),
		Raw:   json.RawMessage(bytes.Clone(raw)),
	}
	if caps, ok := obj["caps"].([]any); ok {
		for _, c := range caps {
			if s, ok := c.(string); ok {
				g.Caps = append(g.Caps, s)
			}
		}
	}
	exp, err := time.Parse(time.RFC3339Nano, str("exp"))
	if err != nil {
		return nil, fmt.Errorf("%w: exp: %v", ErrGrantInvalid, err)
	}
	g.Exp = exp
	iss, err := ParseKey(g.Iss)
	if err != nil {
		return nil, fmt.Errorf("%w: iss: %v", ErrGrantInvalid, err)
	}
	if _, err := ParseKey(g.Sub); err != nil {
		return nil, fmt.Errorf("%w: sub: %v", ErrGrantInvalid, err)
	}
	sig, err := DecodeB64(g.Sig)
	if err != nil || g.Sig == "" {
		return nil, fmt.Errorf("%w: sig missing or malformed", ErrGrantInvalid)
	}
	delete(obj, "sig")
	canon, err := CanonicalValue(obj)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGrantInvalid, err)
	}
	if !ed25519.Verify(iss, canon, sig) {
		return nil, fmt.Errorf("%w: %v", ErrGrantInvalid, ErrBadSignature)
	}
	if !now.IsZero() && !now.Before(g.Exp) {
		return g, ErrGrantExpired
	}
	return g, nil
}

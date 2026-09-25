package daemon

import (
	"encoding/json"
	"testing"
)

// The agent acting counts as activity; dashboards reading does not.
func TestTouches(t *testing.T) {
	for _, c := range []struct {
		method, params string
		want           bool
	}{
		{"touch", ``, true},
		{"send", `{"peer":"x"}`, true},
		{"state", `{}`, true},
		{"read", `{"inbox":true,"unread":true,"mark":true}`, true},
		{"read", `{"peer":"x","th":"t","limit":1000}`, false},
		{"status", ``, false},
		{"presence", ``, false},
		{"threads", `{}`, false},
		{"blobs", `{}`, false},
		{"mirrored", ``, false},
	} {
		if got := touches(c.method, json.RawMessage(c.params)); got != c.want {
			t.Errorf("touches(%s %s) = %v, want %v", c.method, c.params, got, c.want)
		}
	}
}

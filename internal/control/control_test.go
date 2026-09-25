package control

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type echo struct{}

func (echo) Call(ctx context.Context, method string, params json.RawMessage) (any, error) {
	return map[string]string{"method": method}, nil
}

func (echo) Stream(ctx context.Context, method string, params json.RawMessage, emit func(any) error) (bool, error) {
	return false, nil
}

// TestDeepHome: a home too deep for a Unix socket path still works; the
// socket moves to a short, private location both sides agree on.
func TestDeepHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), strings.Repeat("deep/", 30))
	short, err := SocketPath(t.TempDir())
	if err != nil || !strings.HasSuffix(short, SocketName) {
		t.Fatalf("short home: %q, %v", short, err)
	}
	long, err := SocketPath(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(long) > maxSocketPath || strings.HasPrefix(long, home) {
		t.Fatalf("deep home socket %q", long)
	}
	if again, _ := SocketPath(home); again != long {
		t.Fatalf("socket path not stable: %q then %q", long, again)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, home, echo{})
	c := &Client{Home: home}
	deadline := time.Now().Add(5 * time.Second)
	for !c.Running() {
		if time.Now().After(deadline) {
			t.Fatal("server never came up")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var out map[string]string
	if err := c.Call(ctx, "status", nil, &out); err != nil || out["method"] != "status" {
		t.Fatalf("call: %v %v", out, err)
	}
	// A client whose environment would put the socket elsewhere still
	// finds the daemon.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if other, _ := SocketPath(home); other == long {
		t.Fatalf("XDG_RUNTIME_DIR did not move the socket: %q", other)
	}
	if !c.Running() {
		t.Fatal("client with a different XDG_RUNTIME_DIR cannot find the daemon")
	}
}

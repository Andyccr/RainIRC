package stun

import (
	"context"
	"testing"
	"time"
)

func TestBindingRoundTrip(t *testing.T) {
	addr, stop, err := ServeBinding("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, err := Binding(ctx, addr.String())
	if err != nil {
		t.Fatal(err)
	}
	if m.IP == nil || m.Port <= 0 {
		t.Fatalf("mapped=%v", m)
	}
}

func TestParseRejectsShort(t *testing.T) {
	if _, err := parseBinding([]byte("nope"), make([]byte, 12)); err == nil {
		t.Fatal("expected error")
	}
}

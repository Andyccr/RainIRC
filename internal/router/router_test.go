package router

import (
	"sync"
	"testing"
	"time"

	"github.com/Andyccr/RainIRC/internal/protocol"
)

type recSender struct {
	mu     sync.Mutex
	sent   []*protocol.Message
	except []string
}

func (r *recSender) Broadcast(msg *protocol.Message, exceptPeerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, msg)
	r.except = append(r.except, exceptPeerID)
}

func TestDuplicateMessage(t *testing.T) {
	s := &recSender{}
	var handled int
	r := New(s, func(*protocol.Message, string) { handled++ }, time.Hour, 100)
	msg := protocol.NewChat("aa", "A", "#general", "hello", false)
	if !r.Handle(msg, "peerA") {
		t.Fatal("first Handle should accept")
	}
	if r.Handle(msg, "peerB") {
		t.Fatal("duplicate should be ignored")
	}
	if handled != 1 {
		t.Fatalf("handled %d, want 1", handled)
	}
	if len(s.sent) != 1 {
		t.Fatalf("forwarded %d, want 1", len(s.sent))
	}
}

func TestMessageForwarding(t *testing.T) {
	s := &recSender{}
	r := New(s, func(*protocol.Message, string) {}, time.Hour, 100)
	msg := protocol.NewChat("aa", "A", "#general", "hello", false)
	r.Handle(msg, "peerA")
	if len(s.sent) != 1 {
		t.Fatalf("sent %d", len(s.sent))
	}
	if s.except[0] != "peerA" {
		t.Fatalf("should not forward back to sender, except=%s", s.except[0])
	}
}

func TestMessageLoopPrevention(t *testing.T) {
	hops := 0
	var r *Router
	s := broadcastFunc(func(msg *protocol.Message, except string) {
		hops++
		if hops > 20 {
			t.Fatal("message loop")
		}
		// Simulate the message cycling around the mesh.
		r.Handle(msg, "peer"+string(rune('A'+hops)))
	})
	r = New(s, func(*protocol.Message, string) {}, time.Hour, 100)
	msg := protocol.NewChat("aa", "A", "#general", "loop", false)
	r.Inject(msg)
	if hops != 1 {
		t.Fatalf("hops=%d, want 1 (seen-set must stop the flood)", hops)
	}
}

type broadcastFunc func(msg *protocol.Message, exceptPeerID string)

func (f broadcastFunc) Broadcast(msg *protocol.Message, exceptPeerID string) {
	f(msg, exceptPeerID)
}

func TestSeenStoreExpires(t *testing.T) {
	s := NewSeenStore(10*time.Millisecond, 10)
	if !s.Add("a") {
		t.Fatal("first add")
	}
	if s.Add("a") {
		t.Fatal("duplicate")
	}
	time.Sleep(20 * time.Millisecond)
	s.Sweep(time.Now())
	if !s.Add("a") {
		t.Fatal("should be reusable after TTL")
	}
}

func TestSeenStoreBounded(t *testing.T) {
	s := NewSeenStore(time.Hour, 8)
	for i := 0; i < 20; i++ {
		s.Add(string(rune('a' + i)))
	}
	if s.Len() > 8 {
		t.Fatalf("len %d exceeds max", s.Len())
	}
}

func TestStaleMessageDropped(t *testing.T) {
	s := &recSender{}
	r := New(s, func(*protocol.Message, string) {}, time.Hour, 100)
	msg := protocol.NewChat("aa", "A", "#general", "old", false)
	msg.Timestamp = time.Now().Add(-time.Hour).Unix()
	if r.Handle(msg, "peerA") {
		t.Fatal("stale message should be dropped")
	}
	if len(s.sent) != 0 {
		t.Fatal("stale message should not be flooded")
	}
}

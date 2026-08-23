package limiter

import (
	"testing"
	"time"
)

func TestAllowBurst(t *testing.T) {
	w := New(3, time.Second)
	if !w.Allow("a") || !w.Allow("a") || !w.Allow("a") {
		t.Fatal("burst of 3 should pass")
	}
	if w.Allow("a") {
		t.Fatal("4th in window should drop")
	}
	if !w.Allow("b") {
		t.Fatal("other key should pass")
	}
}

func TestWindowExpires(t *testing.T) {
	w := New(1, 20*time.Millisecond)
	if !w.Allow("a") {
		t.Fatal("first")
	}
	time.Sleep(30 * time.Millisecond)
	if !w.Allow("a") {
		t.Fatal("after window")
	}
}

func TestNilAllow(t *testing.T) {
	var w *Window
	if !w.Allow("x") {
		t.Fatal("nil limiter must not block")
	}
}

func TestEvictOldestKey(t *testing.T) {
	w := New(1, time.Hour)
	w.maxKeys = 2
	if !w.Allow("a") || !w.Allow("b") {
		t.Fatal("first two keys")
	}
	if !w.Allow("c") {
		t.Fatal("third key should evict the oldest")
	}
	if !w.Allow("a") {
		t.Fatal("evicted key should be allowed again")
	}
	if w.Allow("c") {
		t.Fatal("kept key should still be at its limit")
	}
}

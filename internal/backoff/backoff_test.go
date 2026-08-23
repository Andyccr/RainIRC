package backoff

import (
	"testing"
	"time"
)

func TestAllowNil(t *testing.T) {
	var tr *Tracker
	if !tr.Allow("x") {
		t.Fatal("nil tracker must not block")
	}
}

func TestExponentialDelay(t *testing.T) {
	now := time.Unix(1_000, 0)
	tr := New(5*time.Second, time.Minute)
	tr.now = func() time.Time { return now }

	if !tr.Allow("a") {
		t.Fatal("first attempt")
	}
	tr.Fail("a")
	if tr.Allow("a") {
		t.Fatal("should wait initial delay")
	}
	now = now.Add(5 * time.Second)
	if !tr.Allow("a") {
		t.Fatal("after 5s")
	}
	tr.Fail("a")
	now = now.Add(5 * time.Second)
	if tr.Allow("a") {
		t.Fatal("second delay is 10s")
	}
	now = now.Add(5 * time.Second)
	if !tr.Allow("a") {
		t.Fatal("after 10s")
	}
	tr.Reset("a")
	if !tr.Allow("a") {
		t.Fatal("reset")
	}
}

func TestCapsAtMax(t *testing.T) {
	now := time.Unix(1_000, 0)
	tr := New(time.Second, 4*time.Second)
	tr.now = func() time.Time { return now }
	for i := 0; i < 6; i++ {
		tr.Fail("a")
	}
	now = now.Add(3 * time.Second)
	if tr.Allow("a") {
		t.Fatal("still inside 4s cap")
	}
	now = now.Add(time.Second)
	if !tr.Allow("a") {
		t.Fatal("cap elapsed")
	}
}

// Package backoff is a per-key exponential delay used for local reconnect
// policy. It is not a network protocol.
package backoff

import (
	"sync"
	"time"
)

// Tracker delays retries for keys that recently failed.
type Tracker struct {
	mu      sync.Mutex
	initial time.Duration
	max     time.Duration
	next    map[string]time.Time
	delay   map[string]time.Duration
	now     func() time.Time
}

func New(initial, max time.Duration) *Tracker {
	if initial <= 0 {
		initial = 5 * time.Second
	}
	if max < initial {
		max = initial
	}
	return &Tracker{
		initial: initial,
		max:     max,
		next:    make(map[string]time.Time),
		delay:   make(map[string]time.Duration),
		now:     time.Now,
	}
}

func (t *Tracker) clock() time.Time {
	if t == nil || t.now == nil {
		return time.Now()
	}
	return t.now()
}

// Allow reports whether a retry for key may run now.
func (t *Tracker) Allow(key string) bool {
	if t == nil || key == "" {
		return true
	}
	now := t.clock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if next, ok := t.next[key]; ok && now.Before(next) {
		return false
	}
	return true
}

// Fail records a failed attempt and pushes the next retry out.
func (t *Tracker) Fail(key string) {
	if t == nil || key == "" {
		return
	}
	now := t.clock()
	t.mu.Lock()
	defer t.mu.Unlock()
	d := t.delay[key]
	if d == 0 {
		d = t.initial
	} else {
		d *= 2
		if d > t.max {
			d = t.max
		}
	}
	t.delay[key] = d
	t.next[key] = now.Add(d)
}

// Reset clears delay after a successful session (or a clean disconnect).
func (t *Tracker) Reset(key string) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	delete(t.next, key)
	delete(t.delay, key)
	t.mu.Unlock()
}

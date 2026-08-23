// Package limiter is a small per-key sliding window used for local policy
// (gossip flood control). It is not a network protocol.
package limiter

import (
	"sync"
	"time"
)

// Window allows at most N events per key inside a sliding interval.
type Window struct {
	mu      sync.Mutex
	limit   int
	span    time.Duration
	hits    map[string][]time.Time
	maxKeys int
}

func New(limit int, span time.Duration) *Window {
	if limit <= 0 {
		limit = 30
	}
	if span <= 0 {
		span = time.Second
	}
	return &Window{
		limit:   limit,
		span:    span,
		hits:    make(map[string][]time.Time),
		maxKeys: 4096,
	}
}

func (w *Window) Allow(key string) bool {
	if w == nil || key == "" {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-w.span)
	w.mu.Lock()
	defer w.mu.Unlock()
	q := w.hits[key]
	i := 0
	for i < len(q) && q[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		q = append([]time.Time(nil), q[i:]...)
	}
	if len(q) >= w.limit {
		w.hits[key] = q
		return false
	}
	if _, ok := w.hits[key]; !ok && len(w.hits) >= w.maxKeys {
		w.dropOldestLocked()
	}
	w.hits[key] = append(q, now)
	return true
}

func (w *Window) dropOldestLocked() {
	var oldest string
	var t time.Time
	first := true
	for k, q := range w.hits {
		if len(q) == 0 {
			delete(w.hits, k)
			continue
		}
		if first || q[0].Before(t) {
			oldest, t, first = k, q[0], false
		}
	}
	if oldest != "" {
		delete(w.hits, oldest)
	}
}

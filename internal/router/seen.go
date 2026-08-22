package router

import (
	"sync"
	"time"
)

// SeenStore is a thread-safe, bounded, TTL cache of message IDs.
type SeenStore struct {
	mu  sync.Mutex
	ttl time.Duration
	max int
	m   map[string]time.Time
}

func NewSeenStore(ttl time.Duration, max int) *SeenStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if max <= 0 {
		max = 50000
	}
	return &SeenStore{
		ttl: ttl,
		max: max,
		m:   make(map[string]time.Time),
	}
}

// Add records id. Returns false if it was already present.
func (s *SeenStore) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; ok {
		return false
	}
	if len(s.m) >= s.max {
		s.sweepLocked(time.Now())
		if len(s.m) >= s.max {
			n := s.max / 10
			if n < 1 {
				n = 1
			}
			s.dropOldestLocked(n)
		}
	}
	s.m[id] = time.Now()
	return true
}

func (s *SeenStore) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[id]
	return ok
}

func (s *SeenStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

func (s *SeenStore) Sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
}

func (s *SeenStore) sweepLocked(now time.Time) {
	cutoff := now.Add(-s.ttl)
	for id, t := range s.m {
		if t.Before(cutoff) {
			delete(s.m, id)
		}
	}
}

func (s *SeenStore) dropOldestLocked(n int) {
	// Simple pass: drop the first n entries older than the rest by scanning.
	type pair struct {
		id string
		t  time.Time
	}
	oldest := make([]pair, 0, n)
	for id, t := range s.m {
		if len(oldest) < n {
			oldest = append(oldest, pair{id, t})
			continue
		}
		// replace the newest among oldest if this is older
		idx := 0
		for i := 1; i < len(oldest); i++ {
			if oldest[i].t.After(oldest[idx].t) {
				idx = i
			}
		}
		if t.Before(oldest[idx].t) {
			oldest[idx] = pair{id, t}
		}
	}
	for _, p := range oldest {
		delete(s.m, p.id)
	}
}

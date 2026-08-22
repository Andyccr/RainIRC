package router

import (
	"sync"
	"time"
)

// SeenStore is a thread-safe, bounded, TTL cache of message IDs.
// Insertion order is tracked so eviction is O(k) in the number dropped,
// not O(n²) in the cache size.
type SeenStore struct {
	mu  sync.Mutex
	ttl time.Duration
	max int
	m   map[string]time.Time
	q   []string
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
		m:   make(map[string]time.Time, max),
		q:   make([]string, 0, max),
	}
}

// Add records id. Returns false if it was already present.
func (s *SeenStore) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; ok {
		return false
	}
	now := time.Now()
	if len(s.m) >= s.max {
		s.sweepLocked(now)
		for len(s.m) >= s.max && len(s.q) > 0 {
			old := s.q[0]
			s.q = s.q[1:]
			delete(s.m, old)
		}
	}
	s.m[id] = now
	s.q = append(s.q, id)
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
	nq := s.q[:0]
	for _, id := range s.q {
		t, ok := s.m[id]
		if !ok || t.Before(cutoff) {
			delete(s.m, id)
			continue
		}
		nq = append(nq, id)
	}
	s.q = nq
}

// Package store holds the in-memory repositories: a generic concurrent map and
// the per-user stats. The Map is the seam a database would slot behind.
package store

import (
	"sync"
	"sync/atomic"
)

// Map is a goroutine-safe map[string]V.
type Map[V any] struct {
	mu sync.RWMutex
	m  map[string]V
}

func NewMap[V any]() *Map[V] { return &Map[V]{m: make(map[string]V)} }

func (s *Map[V]) Get(key string) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

func (s *Map[V]) Put(key string, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

// Delete removes key and reports whether it was present, so the caller can pair
// a removal with exactly one side effect (e.g. releasing a capacity slot).
func (s *Map[V]) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	delete(s.m, key)
	return ok
}

func (s *Map[V]) GetOrCreate(key string, create func() V) V {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[key]; ok {
		return v
	}
	v := create()
	s.m[key] = v
	return v
}

func (s *Map[V]) Range(f func(key string, value V) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.m {
		if !f(k, v) {
			return
		}
	}
}

// Stats is a snapshot of one user's record.
type Stats struct {
	Wins   int64
	Losses int64
	Draws  int64
}

type UserStats struct {
	UserID string
	Stats
}

type counters struct {
	wins   atomic.Int64
	losses atomic.Int64
	draws  atomic.Int64
}

// StatsStore keeps win/loss/draw counters per user, with lock-free increments.
type StatsStore struct {
	byUser *Map[*counters]
}

func NewStatsStore() *StatsStore { return &StatsStore{byUser: NewMap[*counters]()} }

func (s *StatsStore) countersFor(userID string) *counters {
	return s.byUser.GetOrCreate(userID, func() *counters { return &counters{} })
}

func (s *StatsStore) AddWin(userID string)  { s.countersFor(userID).wins.Add(1) }
func (s *StatsStore) AddLoss(userID string) { s.countersFor(userID).losses.Add(1) }
func (s *StatsStore) AddDraw(userID string) { s.countersFor(userID).draws.Add(1) }

// Get reads a user's record without creating one, so it is a pure read — callers
// like GetStats stay side-effect-free and may be served over GET.
func (s *StatsStore) Get(userID string) Stats {
	c, ok := s.byUser.Get(userID)
	if !ok {
		return Stats{}
	}
	return Stats{Wins: c.wins.Load(), Losses: c.losses.Load(), Draws: c.draws.Load()}
}

func (s *StatsStore) All() []UserStats {
	var out []UserStats
	s.byUser.Range(func(userID string, c *counters) bool {
		out = append(out, UserStats{
			UserID: userID,
			Stats:  Stats{Wins: c.wins.Load(), Losses: c.losses.Load(), Draws: c.draws.Load()},
		})
		return true
	})
	return out
}

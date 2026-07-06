package lobby

import (
	"errors"
	"sync"
	"time"

	"github.com/vk496/tictactoe_project/api/gen/tictactoev1"
	"github.com/vk496/tictactoe_project/internal/config"
)

// The wire status the client sees is the source of truth for a match's state, so
// we store the generated tictactoe.v1.GameStatus directly rather than maintaining
// a parallel enum and a mapping. A match runs PENDING -> ASSIGNED -> ABORTED; the
// two purely internal distinctions the lobby needs are derived from other fields
// instead of new enum values:
//   - a PENDING match with an Opponent set is "join in flight" (locked, not
//     joinable) — externally still PENDING;
//   - an ASSIGNED match that has finished keeps routing during the result linger
//     but no longer occupies its players — externally still ASSIGNED.
const (
	statusPending  = tictactoev1.GameStatus_GAME_STATUS_PENDING
	statusAssigned = tictactoev1.GameStatus_GAME_STATUS_ASSIGNED
	statusAborted  = tictactoev1.GameStatus_GAME_STATUS_ABORTED
)

type Match struct {
	ID         string
	Creator    string
	Opponent   string
	Status     tictactoev1.GameStatus
	WorkerAddr string
	CreatedAt  time.Time
	AssignedAt time.Time
	Config     config.Game
	finished   bool
}

// joinable reports whether a second player may still join this match.
func (m *Match) joinable() bool {
	return m.Status == statusPending && m.Opponent == ""
}

var (
	ErrGameNotFound     = errors.New("game not found")
	ErrGameNotAvailable = errors.New("game is not available to join")
	ErrCannotJoinOwn    = errors.New("cannot join your own game")
	ErrAlreadyInGame    = errors.New("user is already in a game")
)

type Matchmaker struct {
	mu      sync.Mutex
	matches map[string]*Match
	active  map[string]string // user id -> the id of their current (non-finished) game
	now     func() time.Time
}

func NewMatchmaker() *Matchmaker {
	return &Matchmaker{
		matches: make(map[string]*Match),
		active:  make(map[string]string),
		now:     time.Now,
	}
}

func (m *Matchmaker) Create(creator, id string, cfg config.Game) (Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, busy := m.active[creator]; busy {
		return Match{}, ErrAlreadyInGame
	}
	match := &Match{
		ID:        id,
		Creator:   creator,
		Status:    statusPending,
		CreatedAt: m.now(),
		Config:    cfg,
	}
	m.matches[id] = match
	m.active[creator] = id
	return *match, nil
}

func (m *Matchmaker) ListPending(limit int) []Match {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collect((*Match).joinable, limit)
}

func (m *Matchmaker) ListActive(limit int) []Match {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collect(func(mt *Match) bool { return mt.Status == statusAssigned && !mt.finished }, limit)
}

func (m *Matchmaker) collect(match func(*Match) bool, limit int) []Match {
	out := make([]Match, 0, limit)
	for _, mt := range m.matches {
		if !match(mt) {
			continue
		}
		out = append(out, *mt)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func (m *Matchmaker) BeginJoin(id, opponent string) (Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	match, ok := m.matches[id]
	switch {
	case !ok:
		return Match{}, ErrGameNotFound
	case !match.joinable():
		return Match{}, ErrGameNotAvailable
	case match.Creator == opponent:
		return Match{}, ErrCannotJoinOwn
	}
	if _, busy := m.active[opponent]; busy {
		return Match{}, ErrAlreadyInGame
	}
	// Lock the join in flight: still PENDING on the wire, but no longer joinable.
	match.Opponent = opponent
	m.active[opponent] = id
	return *match, nil
}

func (m *Matchmaker) CompleteJoin(id, workerAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if match, ok := m.matches[id]; ok {
		match.Status = statusAssigned
		match.WorkerAddr = workerAddr
		match.AssignedAt = m.now()
	}
}

func (m *Matchmaker) FailJoin(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if match, ok := m.matches[id]; ok {
		delete(m.active, match.Opponent)
		match.Opponent = "" // back to joinable PENDING
	}
}

func (m *Matchmaker) Finish(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if match, ok := m.matches[id]; ok {
		m.release(match)
		match.finished = true // stays ASSIGNED so the result is still routable
	}
}

// ReapStale aborts non-finished games whose last activity is older than ttl —
// pending games nobody joined, and assigned games that never reported a result
// (e.g. an abandoned tab or a lost worker) — freeing their players. Returns how
// many were reaped.
func (m *Matchmaker) ReapStale(ttl time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	deadline := m.now().Add(-ttl)
	reaped := 0
	for _, match := range m.matches {
		var stale bool
		switch {
		case match.Status == statusPending:
			stale = match.CreatedAt.Before(deadline)
		case match.Status == statusAssigned && !match.finished:
			stale = match.AssignedAt.Before(deadline)
		}
		if stale {
			m.release(match)
			match.Status = statusAborted
			reaped++
		}
	}
	return reaped
}

func (m *Matchmaker) release(match *Match) {
	delete(m.active, match.Creator)
	if match.Opponent != "" {
		delete(m.active, match.Opponent)
	}
}

func (m *Matchmaker) Get(id string) (Match, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	match, ok := m.matches[id]
	if !ok {
		return Match{}, false
	}
	return *match, true
}

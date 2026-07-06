package lobby

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/vk496/tictactoe_project/api/gen/tictactoev1"
	"github.com/vk496/tictactoe_project/internal/config"
	"github.com/vk496/tictactoe_project/internal/store"
)

type fakePublisher struct{}

func (fakePublisher) Publish(context.Context, string, any) error { return nil }

func defaults() config.Game {
	g, _ := config.NewGame()
	return g
}

// ADR 5: a user may hold only one non-finished game; finishing frees them.
func TestOneGamePerUser(t *testing.T) {
	m := NewMatchmaker()
	cfg := defaults()

	if _, err := m.Create("amy", "g1", cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("amy", "g2", cfg); !errors.Is(err, ErrAlreadyInGame) {
		t.Fatalf("second create: want ErrAlreadyInGame, got %v", err)
	}
	if _, err := m.BeginJoin("g1", "ben"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("ben", "g3", cfg); !errors.Is(err, ErrAlreadyInGame) {
		t.Fatalf("opponent create: want ErrAlreadyInGame, got %v", err)
	}
	m.Finish("g1")
	if _, err := m.Create("amy", "g4", cfg); err != nil {
		t.Fatalf("after finish, amy should be free: %v", err)
	}
}

// ADR 6: joining is race-free — of many racing joiners, exactly one wins.
func TestConcurrentJoinExactlyOneWins(t *testing.T) {
	m := NewMatchmaker()
	if _, err := m.Create("host", "g1", defaults()); err != nil {
		t.Fatal(err)
	}

	var wins int32
	var wg sync.WaitGroup
	for _, opponent := range []string{"a", "b", "c", "d", "e"} {
		wg.Add(1)
		go func(o string) {
			defer wg.Done()
			if _, err := m.BeginJoin("g1", o); err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}(opponent)
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("want exactly one winner, got %d", wins)
	}
}

// ADR 5: the lobby reaps games idle past the TTL and frees their players.
func TestReapStaleFreesPlayers(t *testing.T) {
	m := NewMatchmaker()
	clock := time.Now()
	m.now = func() time.Time { return clock }

	if _, err := m.Create("amy", "g1", defaults()); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Hour) // now idle past the 1h ttl below

	if n := m.ReapStale(time.Hour); n != 1 {
		t.Fatalf("want 1 reaped, got %d", n)
	}
	if _, err := m.Create("amy", "g2", defaults()); err != nil {
		t.Fatalf("after reap, amy should be free: %v", err)
	}
}

// ADR 3: CreateGame rejects a configuration where win length exceeds board size.
func TestCreateGameRejectsBadConfig(t *testing.T) {
	svc := NewService(defaults(), NewMatchmaker(), store.NewStatsStore(), fakePublisher{}, "", nil, time.Hour)
	_, err := svc.CreateGame(context.Background(),
		connect.NewRequest(&tictactoev1.CreateGameRequest{UserId: "amy", BoardSize: 3, WinLength: 5}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}

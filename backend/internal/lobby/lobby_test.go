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

	create := func(user, id string) error {
		_, err := m.Create(user, id, defaults())
		return err
	}
	mustCreate := func(user, id string) {
		t.Helper()
		if err := create(user, id); err != nil {
			t.Fatalf("Create(%s): %v", user, err)
		}
	}
	wantBusy := func(user, id string) {
		t.Helper()
		if err := create(user, id); !errors.Is(err, ErrAlreadyInGame) {
			t.Fatalf("Create(%s) while already in a game: want ErrAlreadyInGame, got %v", user, err)
		}
	}

	mustCreate("amy", "g1")
	wantBusy("amy", "g2") // amy already hosts g1
	if _, err := m.BeginJoin("g1", "ben"); err != nil {
		t.Fatalf("BeginJoin: %v", err)
	}
	wantBusy("ben", "g3") // ben is now the opponent in g1
	m.Finish("g1")
	mustCreate("amy", "g4") // finishing g1 freed amy
}

// ADR 6: joining is race-free — of many racing joiners, exactly one wins.
func TestConcurrentJoinExactlyOneWins(t *testing.T) {
	m := NewMatchmaker()
	if _, err := m.Create("host", "g1", defaults()); err != nil {
		t.Fatal(err)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	for _, opponent := range []string{"a", "b", "c", "d", "e"} {
		wg.Go(func() {
			if _, err := m.BeginJoin("g1", opponent); err == nil {
				wins.Add(1)
			}
		})
	}
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("winners = %d; want exactly 1", got)
	}
}

// ADR 5: the lobby reaps games idle past the TTL and frees their players.
func TestReapStaleFreesPlayers(t *testing.T) {
	m := NewMatchmaker()
	now := time.Now()
	m.now = func() time.Time { return now }

	if _, err := m.Create("amy", "g1", defaults()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour) // amy's game is now idle past the 1h TTL

	if reaped := m.ReapStale(time.Hour); reaped != 1 {
		t.Fatalf("reaped = %d; want 1", reaped)
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
		t.Fatalf("CreateGame with bad config: want InvalidArgument, got %v", err)
	}
}

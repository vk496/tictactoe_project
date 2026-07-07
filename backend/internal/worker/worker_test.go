package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vk496/tictactoe_project/internal/rabbitmq"
)

type fakePublisher struct{}

func (fakePublisher) Publish(context.Context, string, any) error { return nil }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ADR 8: a worker holds a bounded number of games and refuses more when full.
func TestWorkerCapacity(t *testing.T) {
	svc := NewService(fakePublisher{}, "http://w:8080", 1, discardLogger())
	svc.acquireTimeout = 10 * time.Millisecond // fail fast instead of waiting a full second

	start := func(id string) error {
		body, _ := json.Marshal(rabbitmq.StartGame{
			GameRef:   rabbitmq.GameRef{GameID: id},
			Players:   rabbitmq.Players{PlayerX: "a", PlayerO: "b"},
			BoardSize: 3,
			WinLength: 3,
		})
		return svc.HandleStartGame(context.Background(), body)
	}

	if err := start("g1"); err != nil {
		t.Fatalf("first game (capacity 1) should be accepted: %v", err)
	}
	if err := start("g2"); !errors.Is(err, errAtCapacity) {
		t.Fatalf("second game: want errAtCapacity, got %v", err)
	}
}

// Command tictactoe is one binary with three roles. Run it as:
//
//	tictactoe lobby     # the authority: matchmaking + stats
//	tictactoe worker    # holds live games, processes moves
//	tictactoe gateway   # the edge: routes moves to the owning worker
//
// Each role's wiring lives in its own package's Run function.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vk496/tictactoe_project/internal/gateway"
	"github.com/vk496/tictactoe_project/internal/lobby"
	"github.com/vk496/tictactoe_project/internal/worker"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	roles := map[string]func(context.Context, *slog.Logger) error{
		"lobby":   lobby.Run,
		"worker":  worker.Run,
		"gateway": gateway.Run,
	}

	if len(os.Args) < 2 || roles[os.Args[1]] == nil {
		logger.Error("usage: tictactoe <lobby|worker|gateway>")
		os.Exit(2)
	}
	role := os.Args[1]

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := roles[role](ctx, logger); err != nil {
		logger.Error("exited with error", "role", role, "error", err)
		os.Exit(1)
	}
}

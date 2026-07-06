package config_test

import (
	"testing"

	"github.com/vk496/tictactoe_project/internal/config"
)

// ADR 3: NewGame validates the rules — win length must be >= 1 and <= board size.
func TestGameConfigValidation(t *testing.T) {
	if _, err := config.NewGame(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
	if _, err := config.NewGame(config.WithBoardSize(5), config.WithWinLength(4)); err != nil {
		t.Fatalf("5x5/K4 should be valid: %v", err)
	}
	if _, err := config.NewGame(config.WithBoardSize(3), config.WithWinLength(5)); err == nil {
		t.Fatal("win length > board size should be rejected")
	}
	if _, err := config.NewGame(config.WithBoardSize(2)); err == nil {
		t.Fatal("board size < 3 should be rejected (1 or 2 is not a real game)")
	}
}

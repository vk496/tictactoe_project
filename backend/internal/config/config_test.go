package config_test

import (
	"testing"

	"github.com/vk496/tictactoe_project/internal/config"
)

// ADR 3: NewGame validates the rules — the board is at least 3×3 and the win
// length is between 1 and the board size.
func TestGameConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []config.Option
		wantErr bool
	}{
		{"defaults", nil, false},
		{"5x5, win 4", []config.Option{config.WithBoardSize(5), config.WithWinLength(4)}, false},
		{"win length exceeds board", []config.Option{config.WithBoardSize(3), config.WithWinLength(5)}, true},
		{"board smaller than 3", []config.Option{config.WithBoardSize(2)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.NewGame(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewGame() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

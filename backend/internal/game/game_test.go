package game_test

import (
	"testing"

	"github.com/vk496/tictactoe_project/internal/game"
)

type move struct {
	user string
	r, c int
}

func newGame(size, win int) *game.Game {
	return game.New("x", "o", game.NewBoard(size), game.NewLineChecker(win))
}

func play(t *testing.T, g *game.Game, moves ...move) {
	t.Helper()
	for _, m := range moves {
		if err := g.ApplyMove(m.user, m.r, m.c); err != nil {
			t.Fatalf("ApplyMove(%s, %d, %d): %v", m.user, m.r, m.c, err)
		}
	}
}

// ADR 3: board size and win length are configurable — K in a row on an N×N board
// wins, and a full board with no line is a draw.
func TestConfigurableBoardAndWin(t *testing.T) {
	t.Run("K in a row wins (5x5, K=3)", func(t *testing.T) {
		g := newGame(5, 3)
		play(t, g, move{"x", 2, 0}, move{"o", 0, 0}, move{"x", 2, 1}, move{"o", 0, 1}, move{"x", 2, 2})
		if g.Status() != game.Won || g.WinnerID() != "x" {
			t.Fatalf("want x won, got status=%v winner=%q", g.Status(), g.WinnerID())
		}
	})

	t.Run("a full board with no line draws (3x3)", func(t *testing.T) {
		g := newGame(3, 3)
		play(t, g,
			move{"x", 0, 0}, move{"o", 0, 1}, move{"x", 0, 2},
			move{"o", 1, 1}, move{"x", 1, 0}, move{"o", 1, 2},
			move{"x", 2, 1}, move{"o", 2, 0}, move{"x", 2, 2},
		)
		if g.Status() != game.Drawn {
			t.Fatalf("want draw, got %v", g.Status())
		}
	})
}

// ADR 4: the domain enforces turn-based integrity with typed errors, and no move
// is accepted once the game is over.
func TestApplyMoveRules(t *testing.T) {
	t.Run("illegal moves return typed errors", func(t *testing.T) {
		g := newGame(3, 3) // x moves first

		illegal := []struct {
			name string
			m    move
			want error
		}{
			{"out of turn", move{"o", 0, 0}, game.ErrNotYourTurn},
			{"not a player", move{"stranger", 0, 0}, game.ErrNotAPlayer},
			{"off the board", move{"x", 9, 9}, game.ErrOutOfBounds},
		}
		for _, tc := range illegal {
			if err := g.ApplyMove(tc.m.user, tc.m.r, tc.m.c); err != tc.want {
				t.Errorf("%s: want %v, got %v", tc.name, tc.want, err)
			}
		}

		play(t, g, move{"x", 0, 0})
		if err := g.ApplyMove("o", 0, 0); err != game.ErrCellTaken {
			t.Errorf("occupied cell: want ErrCellTaken, got %v", err)
		}
	})

	t.Run("no move is accepted after the game is over", func(t *testing.T) {
		g := newGame(3, 3)
		play(t, g, move{"x", 0, 0}, move{"o", 1, 0}, move{"x", 0, 1}, move{"o", 1, 1}, move{"x", 0, 2})
		if g.Status() != game.Won {
			t.Fatalf("want won, got %v", g.Status())
		}
		if err := g.ApplyMove("o", 2, 2); err != game.ErrGameOver {
			t.Errorf("after win: want ErrGameOver, got %v", err)
		}
	})
}

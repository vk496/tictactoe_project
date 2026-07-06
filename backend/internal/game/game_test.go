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
			t.Fatalf("ApplyMove(%s,%d,%d): %v", m.user, m.r, m.c, err)
		}
	}
}

// ADR 3: board size and win length are configurable — K-in-a-row on an N×N board
// (with K < N) wins, and a full board with no line is a draw.
func TestConfigurableBoardAndWin(t *testing.T) {
	// 3 in a row on a 5×5 board.
	g := newGame(5, 3)
	play(t, g, move{"x", 2, 0}, move{"o", 0, 0}, move{"x", 2, 1}, move{"o", 0, 1}, move{"x", 2, 2})
	if g.Status() != game.Won || g.WinnerID() != "x" {
		t.Fatalf("5x5/K3: want x won, got status=%v winner=%q", g.Status(), g.WinnerID())
	}

	// Full 3×3 board with no line -> draw.
	d := newGame(3, 3)
	play(t, d,
		move{"x", 0, 0}, move{"o", 0, 1}, move{"x", 0, 2},
		move{"o", 1, 1}, move{"x", 1, 0}, move{"o", 1, 2},
		move{"x", 2, 1}, move{"o", 2, 0}, move{"x", 2, 2},
	)
	if d.Status() != game.Drawn {
		t.Fatalf("want draw, got %v", d.Status())
	}
}

// ADR 4: the domain enforces turn-based integrity with typed errors, and no move
// is accepted once the game is over.
func TestApplyMoveRules(t *testing.T) {
	g := newGame(3, 3) // x moves first

	if err := g.ApplyMove("o", 0, 0); err != game.ErrNotYourTurn {
		t.Fatalf("o first: want ErrNotYourTurn, got %v", err)
	}
	if err := g.ApplyMove("stranger", 0, 0); err != game.ErrNotAPlayer {
		t.Fatalf("stranger: want ErrNotAPlayer, got %v", err)
	}
	if err := g.ApplyMove("x", 9, 9); err != game.ErrOutOfBounds {
		t.Fatalf("out of bounds: want ErrOutOfBounds, got %v", err)
	}

	play(t, g, move{"x", 0, 0})
	if err := g.ApplyMove("o", 0, 0); err != game.ErrCellTaken {
		t.Fatalf("occupied: want ErrCellTaken, got %v", err)
	}

	// Play x to a win, then any further move is rejected.
	play(t, g, move{"o", 1, 0}, move{"x", 0, 1}, move{"o", 1, 1}, move{"x", 0, 2})
	if g.Status() != game.Won {
		t.Fatalf("want won, got %v", g.Status())
	}
	if err := g.ApplyMove("o", 2, 2); err != game.ErrGameOver {
		t.Fatalf("after win: want ErrGameOver, got %v", err)
	}
}

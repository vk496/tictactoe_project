// Package game is the tic-tac-toe domain: the board, the win/draw rule, and the
// game state machine. It has no I/O; its only dependency is the generated
// protobuf enums (Mark, GameStatus), which it reuses rather than mirror — so the
// domain and the wire share one set of definitions and never need converting.
package game

import (
	"errors"

	"github.com/vk496/tictactoe_project/api/gen/tictactoev1"
)

var (
	ErrGameOver    = errors.New("game is over")
	ErrNotAPlayer  = errors.New("user is not a player in this game")
	ErrNotYourTurn = errors.New("not your turn")
	ErrOutOfBounds = errors.New("move is out of bounds")
	ErrCellTaken   = errors.New("cell is already taken")
)

// Mark is the content of a cell — the shared protobuf enum under a short name.
type Mark = tictactoev1.Mark

const (
	Empty = tictactoev1.Mark_MARK_EMPTY
	X     = tictactoev1.Mark_MARK_X
	O     = tictactoev1.Mark_MARK_O
)

func Opponent(m Mark) Mark {
	if m == X {
		return O
	}
	return X
}

// Board is an N×N grid stored row-major.
type Board struct {
	size  int
	cells []Mark
}

func NewBoard(size int) *Board {
	return &Board{size: size, cells: make([]Mark, size*size)}
}

func (b *Board) Size() int                 { return b.size }
func (b *Board) At(row, col int) Mark      { return b.cells[row*b.size+col] }
func (b *Board) IsEmpty(row, col int) bool { return b.At(row, col) == Empty }
func (b *Board) set(row, col int, m Mark)  { b.cells[row*b.size+col] = m }

func (b *Board) InBounds(row, col int) bool {
	return row >= 0 && row < b.size && col >= 0 && col < b.size
}

func (b *Board) Full() bool {
	for _, c := range b.cells {
		if c == Empty {
			return false
		}
	}
	return true
}

// WinChecker is the win/draw rule — a Strategy so board size and win length are
// configurable. LineChecker looks for winLength marks in a row through the last
// move; deliberately unoptimized, per the brief.
type WinChecker interface {
	Evaluate(b *Board, last Move) Outcome
}

type Outcome uint8

const (
	InProgress Outcome = iota
	Win
	Draw
)

type Move struct {
	Row, Col int
	Mark     Mark
}

type LineChecker struct{ winLength int }

func NewLineChecker(winLength int) LineChecker { return LineChecker{winLength: winLength} }

// axes: horizontal, vertical, and the two diagonals.
var axes = [...][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}

func (c LineChecker) Evaluate(b *Board, last Move) Outcome {
	for _, axis := range axes {
		if c.aligned(b, last, axis[0], axis[1]) >= c.winLength {
			return Win
		}
	}
	if b.Full() {
		return Draw
	}
	return InProgress
}

func (c LineChecker) aligned(b *Board, last Move, dr, dc int) int {
	return 1 + c.run(b, last, dr, dc) + c.run(b, last, -dr, -dc)
}

func (c LineChecker) run(b *Board, last Move, dr, dc int) int {
	count := 0
	for r, col := last.Row+dr, last.Col+dc; b.InBounds(r, col) && b.At(r, col) == last.Mark; r, col = r+dr, col+dc {
		count++
	}
	return count
}

// Status is a Game's lifecycle — the shared protobuf GameStatus under a short
// name. The domain only ever sets the in-play region (Active/Won/Drawn); the
// lobby owns the rest (Pending/Assigned/Aborted) of the same enum.
type Status = tictactoev1.GameStatus

const (
	Active = tictactoev1.GameStatus_GAME_STATUS_ACTIVE
	Won    = tictactoev1.GameStatus_GAME_STATUS_WON
	Drawn  = tictactoev1.GameStatus_GAME_STATUS_DRAWN
)

// Game is the aggregate: it owns the board and enforces the rules. ApplyMove is
// the only mutator — it validates phase, player, turn, bounds and emptiness, and
// returns a typed error rather than panicking.
type Game struct {
	playerX string
	playerO string
	board   *Board
	rules   WinChecker
	turn    Mark
	status  Status
	winner  Mark
}

func New(playerX, playerO string, board *Board, rules WinChecker) *Game {
	return &Game{
		playerX: playerX,
		playerO: playerO,
		board:   board,
		rules:   rules,
		turn:    X,
		status:  Active,
	}
}

func (g *Game) ApplyMove(userID string, row, col int) error {
	if g.status != Active {
		return ErrGameOver
	}
	mark, ok := g.markFor(userID)
	if !ok {
		return ErrNotAPlayer
	}
	if mark != g.turn {
		return ErrNotYourTurn
	}
	if !g.board.InBounds(row, col) {
		return ErrOutOfBounds
	}
	if !g.board.IsEmpty(row, col) {
		return ErrCellTaken
	}

	g.board.set(row, col, mark)
	switch g.rules.Evaluate(g.board, Move{Row: row, Col: col, Mark: mark}) {
	case Win:
		g.status, g.winner = Won, mark
	case Draw:
		g.status = Drawn
	default:
		g.turn = Opponent(g.turn)
	}
	return nil
}

func (g *Game) markFor(userID string) (Mark, bool) {
	switch userID {
	case g.playerX:
		return X, true
	case g.playerO:
		return O, true
	default:
		return Empty, false
	}
}

func (g *Game) Board() *Board   { return g.board }
func (g *Game) Status() Status  { return g.status }
func (g *Game) Turn() Mark      { return g.turn }
func (g *Game) Winner() Mark    { return g.winner }
func (g *Game) PlayerX() string { return g.playerX }
func (g *Game) PlayerO() string { return g.playerO }

func (g *Game) TurnUserID() string {
	if g.turn == X {
		return g.playerX
	}
	return g.playerO
}

func (g *Game) WinnerID() string {
	switch g.winner {
	case X:
		return g.playerX
	case O:
		return g.playerO
	default:
		return ""
	}
}

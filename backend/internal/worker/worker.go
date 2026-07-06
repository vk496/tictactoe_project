package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/vk496/tictactoe_project/api/gen/tictactoev1"
	"github.com/vk496/tictactoe_project/api/gen/tictactoev1/tictactoev1connect"
	"github.com/vk496/tictactoe_project/internal/config"
	"github.com/vk496/tictactoe_project/internal/game"
	"github.com/vk496/tictactoe_project/internal/httpx"
	"github.com/vk496/tictactoe_project/internal/rabbitmq"
	"github.com/vk496/tictactoe_project/internal/store"
)

var _ tictactoev1connect.WorkerHandler = (*Service)(nil)

var (
	errGameNotHere = errors.New("game is not hosted on this worker")
	errAtCapacity  = errors.New("worker is at capacity")
)

// defaultAcquireTimeout bounds how long HandleStartGame waits for a free capacity
// slot before refusing a game. Waiting briefly both lets a slot free up during a
// short burst and throttles the broker's requeue so a full pool does not spin.
// It is a Service field (defaulted here) so a test can shorten it.
const defaultAcquireTimeout = time.Second

// Run consumes StartGame from the broker and serves the Worker API over HTTP.
func Run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	b, err := rabbitmq.Connect(ctx, cfg.AMQPURL)
	if err != nil {
		return fmt.Errorf("connect to broker: %w", err)
	}
	defer b.Close()

	svc := NewService(b, cfg.WorkerAdvertiseAddr, cfg.WorkerMaxGames, logger)
	if err := b.Consume(rabbitmq.QueueStart, svc.HandleStartGame); err != nil {
		return fmt.Errorf("consume start: %w", err)
	}
	go svc.reapLoop(ctx, cfg.WorkerReapInterval, cfg.FinishedLinger, cfg.AbandonedAfter)

	mux := http.NewServeMux()
	mux.Handle(tictactoev1connect.NewWorkerHandler(svc))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	logger.Info("worker starting", "self_addr", cfg.WorkerAdvertiseAddr, "max_games", cfg.WorkerMaxGames)
	return httpx.Serve(ctx, httpx.NewServer(cfg.WorkerListenAddr, mux), logger)
}

type guardedGame struct {
	mu         sync.Mutex
	game       *game.Game
	winLength  int
	lastActive time.Time
}

type Service struct {
	games          *store.Map[*guardedGame]
	slots          chan struct{} // capacity semaphore: holds one token per hosted game
	acquireTimeout time.Duration
	publisher      rabbitmq.Publisher
	selfAddr       string
	logger         *slog.Logger
}

func NewService(publisher rabbitmq.Publisher, selfAddr string, maxGames int, logger *slog.Logger) *Service {
	return &Service{
		games:          store.NewMap[*guardedGame](),
		slots:          make(chan struct{}, maxGames),
		acquireTimeout: defaultAcquireTimeout,
		publisher:      publisher,
		selfAddr:       selfAddr,
		logger:         logger,
	}
}

// HandleStartGame is the RabbitMQ consumer for the start queue. The broker hands
// this worker one game at a time (prefetch = 1): returning nil acks it; returning
// an error nacks it, and the broker offers the game to another worker. So a full
// worker "refuses" a game simply by returning errAtCapacity here.
func (s *Service) HandleStartGame(ctx context.Context, body []byte) error {
	var msg rabbitmq.StartGame
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}

	// Reserve one of the bounded capacity slots. If the worker is already full,
	// refuse the game (the nack above requeues it for another worker).
	if !s.reserveSlot() {
		s.logger.Debug("refused game: at capacity", "game_id", msg.GameID)
		return errAtCapacity
	}

	// The slot is ours: host the game, then tell the lobby where it lives. If that
	// announcement fails, drop the game and release the slot before requeueing.
	s.host(msg)
	if err := s.publisher.Publish(ctx, rabbitmq.QueueAssigned, rabbitmq.GameAssigned{
		GameRef:    rabbitmq.GameRef{GameID: msg.GameID},
		WorkerAddr: s.selfAddr,
	}); err != nil {
		s.evict(msg.GameID)
		return err
	}
	return nil
}

// reserveSlot takes one capacity slot, waiting up to acquireTimeout for one to
// free. It reports whether a slot was acquired.
func (s *Service) reserveSlot() bool {
	select {
	case s.slots <- struct{}{}:
		return true
	case <-time.After(s.acquireTimeout):
		return false
	}
}

// host builds the game and records it in memory. The caller must already hold a
// slot (see reserveSlot).
func (s *Service) host(msg rabbitmq.StartGame) {
	board := game.NewBoard(msg.BoardSize)
	rules := game.NewLineChecker(msg.WinLength)
	g := game.New(msg.PlayerX, msg.PlayerO, board, rules)
	s.games.Put(msg.GameID, &guardedGame{game: g, winLength: msg.WinLength, lastActive: time.Now()})
}

// evict removes a game and releases the capacity slot it held. Deleting from the
// map is the point of truth, so the slot is released exactly once.
func (s *Service) evict(id string) {
	if s.games.Delete(id) {
		<-s.slots
	}
}

// reapLoop frees slots over time by evicting games: finished ones after a short
// linger (both players have seen the result), and ones with no move for a long
// time (an abandoned tab, or a lost peer).
func (s *Service) reapLoop(ctx context.Context, interval, finishedLinger, abandonedAfter time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			var stale []string
			s.games.Range(func(id string, gg *guardedGame) bool {
				gg.mu.Lock()
				idle := now.Sub(gg.lastActive)
				done := gg.game.Status() != game.Active
				gg.mu.Unlock()
				if (done && idle > finishedLinger) || (!done && idle > abandonedAfter) {
					stale = append(stale, id)
				}
				return true
			})
			for _, id := range stale {
				s.evict(id)
			}
		}
	}
}

func (s *Service) MakeMove(_ context.Context, req *connect.Request[tictactoev1.MakeMoveRequest]) (*connect.Response[tictactoev1.GameView], error) {
	gg, ok := s.games.Get(req.Msg.GameId)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errGameNotHere)
	}

	gg.mu.Lock()
	if err := gg.game.ApplyMove(req.Msg.UserId, int(req.Msg.Row), int(req.Msg.Col)); err != nil {
		gg.mu.Unlock()
		return nil, moveError(err)
	}
	gg.lastActive = time.Now()
	view := toView(req.Msg.GameId, gg)
	res, finished := terminal(req.Msg.GameId, gg.game)
	gg.mu.Unlock()

	if finished {
		s.report(res)
	}
	return connect.NewResponse(view), nil
}

func (s *Service) GetBoard(_ context.Context, req *connect.Request[tictactoev1.GetBoardRequest]) (*connect.Response[tictactoev1.GameView], error) {
	gg, ok := s.games.Get(req.Msg.GameId)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errGameNotHere)
	}
	gg.mu.Lock()
	view := toView(req.Msg.GameId, gg)
	gg.mu.Unlock()
	return connect.NewResponse(view), nil
}

func terminal(gameID string, g *game.Game) (rabbitmq.GameResult, bool) {
	ref := rabbitmq.GameRef{GameID: gameID}
	players := rabbitmq.Players{PlayerX: g.PlayerX(), PlayerO: g.PlayerO()}
	switch g.Status() {
	case game.Won:
		return rabbitmq.GameResult{GameRef: ref, Players: players, WinnerID: g.WinnerID()}, true
	case game.Drawn:
		return rabbitmq.GameResult{GameRef: ref, Players: players, Draw: true}, true
	default:
		return rabbitmq.GameResult{}, false
	}
}

func (s *Service) report(res rabbitmq.GameResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.publisher.Publish(ctx, rabbitmq.QueueResult, res); err != nil {
		s.logger.Error("publish result failed", "game_id", res.GameID, "error", err)
	}
}

func moveError(err error) error {
	switch {
	case errors.Is(err, game.ErrNotAPlayer):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, game.ErrGameOver), errors.Is(err, game.ErrNotYourTurn):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, game.ErrCellTaken):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, game.ErrOutOfBounds):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func toView(id string, gg *guardedGame) *tictactoev1.GameView {
	g := gg.game
	b := g.Board()
	rows := make([]*tictactoev1.Row, b.Size())
	for r := 0; r < b.Size(); r++ {
		cells := make([]tictactoev1.Mark, b.Size())
		for c := 0; c < b.Size(); c++ {
			cells[c] = b.At(r, c) // game.Mark is the protobuf Mark — no conversion
		}
		rows[r] = &tictactoev1.Row{Cells: cells}
	}
	return &tictactoev1.GameView{
		GameId:     id,
		Status:     g.Status(), // game.Status is the protobuf GameStatus — no conversion
		Rows:       rows,
		BoardSize:  int32(b.Size()),
		WinLength:  int32(gg.winLength),
		PlayerX:    g.PlayerX(),
		PlayerO:    g.PlayerO(),
		TurnUserId: g.TurnUserID(),
		WinnerId:   g.WinnerID(),
	}
}

package lobby

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"connectrpc.com/connect"

	"github.com/vk496/tictactoe_project/api/gen/tictactoev1"
	"github.com/vk496/tictactoe_project/api/gen/tictactoev1/tictactoev1connect"
	"github.com/vk496/tictactoe_project/internal/config"
	"github.com/vk496/tictactoe_project/internal/httpx"
	"github.com/vk496/tictactoe_project/internal/rabbitmq"
	"github.com/vk496/tictactoe_project/internal/routing"
	"github.com/vk496/tictactoe_project/internal/store"
)

// Run wires the lobby: broker consumers for GameAssigned/GameResult, a reaper
// for stale games, and the Lobby API over HTTP.
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

	svc := NewService(
		cfg.DefaultGame, NewMatchmaker(), store.NewStatsStore(), b,
		cfg.GatewayURL, []byte(cfg.RoutingSecret), cfg.RoutingTTL,
	)
	if err := b.Consume(rabbitmq.QueueAssigned, svc.HandleAssigned); err != nil {
		return fmt.Errorf("consume assigned: %w", err)
	}
	if err := b.Consume(rabbitmq.QueueResult, svc.HandleResult); err != nil {
		return fmt.Errorf("consume result: %w", err)
	}
	go svc.reapLoop(ctx, cfg.ReapInterval, cfg.GameTTL, logger)

	mux := http.NewServeMux()
	mux.Handle(tictactoev1connect.NewLobbyHandler(svc))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	logger.Info("lobby starting", "default_board_size", cfg.DefaultGame.BoardSize, "default_win_length", cfg.DefaultGame.WinLength)
	return httpx.Serve(ctx, httpx.NewServer(cfg.LobbyListenAddr, mux), logger)
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Service) reapLoop(ctx context.Context, interval, ttl time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := s.ReapStaleGames(ttl); n > 0 {
				logger.Info("reaped stale games", "count", n)
			}
		}
	}
}

var _ tictactoev1connect.LobbyHandler = (*Service)(nil)

type Stats interface {
	Get(userID string) store.Stats
	All() []store.UserStats
	AddWin(userID string)
	AddLoss(userID string)
	AddDraw(userID string)
}

type Service struct {
	defaults    config.Game
	matches     *Matchmaker
	stats       Stats
	publisher   rabbitmq.Publisher
	gatewayURL  string
	routeSecret []byte
	routeTTL    time.Duration
}

func NewService(defaults config.Game, matches *Matchmaker, stats Stats, publisher rabbitmq.Publisher, gatewayURL string, routeSecret []byte, routeTTL time.Duration) *Service {
	return &Service{
		defaults:    defaults,
		matches:     matches,
		stats:       stats,
		publisher:   publisher,
		gatewayURL:  gatewayURL,
		routeSecret: routeSecret,
		routeTTL:    routeTTL,
	}
}

func (s *Service) CreateGame(_ context.Context, req *connect.Request[tictactoev1.CreateGameRequest]) (*connect.Response[tictactoev1.CreateGameResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}
	cfg, err := s.gameConfig(req.Msg.BoardSize, req.Msg.WinLength)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	m, err := s.matches.Create(req.Msg.UserId, newID(), cfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&tictactoev1.CreateGameResponse{
		GameId: m.ID,
		Status: m.Status,
	}), nil
}

// gameConfig resolves the per-game board size and win length, falling back to the
// server defaults when a field is left at zero, and validates the combination.
func (s *Service) gameConfig(boardSize, winLength int32) (config.Game, error) {
	size := s.defaults.BoardSize
	if boardSize > 0 {
		size = int(boardSize)
	}
	length := s.defaults.WinLength
	if winLength > 0 {
		length = int(winLength)
	}
	return config.NewGame(config.WithBoardSize(size), config.WithWinLength(length))
}

func (s *Service) ListPendingGames(_ context.Context, req *connect.Request[tictactoev1.ListPendingGamesRequest]) (*connect.Response[tictactoev1.ListPendingGamesResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50
	}
	pending := s.matches.ListPending(limit)
	games := make([]*tictactoev1.PendingGame, 0, len(pending))
	for _, m := range pending {
		games = append(games, &tictactoev1.PendingGame{
			GameId:        m.ID,
			CreatorId:     m.Creator,
			CreatedAtUnix: m.CreatedAt.Unix(),
			BoardSize:     int32(m.Config.BoardSize),
			WinLength:     int32(m.Config.WinLength),
		})
	}
	return connect.NewResponse(&tictactoev1.ListPendingGamesResponse{Games: games}), nil
}

func (s *Service) ListActiveGames(_ context.Context, req *connect.Request[tictactoev1.ListActiveGamesRequest]) (*connect.Response[tictactoev1.ListActiveGamesResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50
	}
	active := s.matches.ListActive(limit)
	games := make([]*tictactoev1.ActiveGame, 0, len(active))
	for _, m := range active {
		games = append(games, &tictactoev1.ActiveGame{
			GameId:        m.ID,
			PlayerX:       m.Creator,
			PlayerO:       m.Opponent,
			BoardSize:     int32(m.Config.BoardSize),
			WinLength:     int32(m.Config.WinLength),
			StartedAtUnix: m.AssignedAt.Unix(),
		})
	}
	return connect.NewResponse(&tictactoev1.ListActiveGamesResponse{Games: games}), nil
}

func (s *Service) Leaderboard(_ context.Context, req *connect.Request[tictactoev1.LeaderboardRequest]) (*connect.Response[tictactoev1.LeaderboardResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10
	}
	all := s.stats.All()
	played := all[:0]
	for _, u := range all {
		if u.Wins+u.Losses+u.Draws > 0 {
			played = append(played, u)
		}
	}
	all = played
	sort.Slice(all, func(i, j int) bool {
		if all[i].Wins != all[j].Wins {
			return all[i].Wins > all[j].Wins
		}
		if all[i].Losses != all[j].Losses {
			return all[i].Losses < all[j].Losses
		}
		return all[i].UserID < all[j].UserID
	})
	if len(all) > limit {
		all = all[:limit]
	}
	entries := make([]*tictactoev1.LeaderboardEntry, 0, len(all))
	for _, u := range all {
		entries = append(entries, &tictactoev1.LeaderboardEntry{
			UserId: u.UserID,
			Wins:   u.Wins,
			Losses: u.Losses,
			Draws:  u.Draws,
		})
	}
	return connect.NewResponse(&tictactoev1.LeaderboardResponse{Entries: entries}), nil
}

// ReapStaleGames aborts games that have been idle past ttl (abandoned lobby
// entries and games that never reported a result), freeing their players.
func (s *Service) ReapStaleGames(ttl time.Duration) int {
	return s.matches.ReapStale(ttl)
}

func (s *Service) JoinGame(ctx context.Context, req *connect.Request[tictactoev1.JoinGameRequest]) (*connect.Response[tictactoev1.JoinGameResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	m, err := s.matches.BeginJoin(req.Msg.GameId, req.Msg.UserId)
	if err != nil {
		return nil, joinError(err)
	}

	start := rabbitmq.StartGame{
		GameRef:   rabbitmq.GameRef{GameID: m.ID},
		Players:   rabbitmq.Players{PlayerX: m.Creator, PlayerO: m.Opponent},
		BoardSize: m.Config.BoardSize,
		WinLength: m.Config.WinLength,
	}
	if err := s.publisher.Publish(ctx, rabbitmq.QueueStart, start); err != nil {
		s.matches.FailJoin(m.ID)
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("enqueue start game: %w", err))
	}

	return connect.NewResponse(&tictactoev1.JoinGameResponse{
		GameId: m.ID,
		Status: m.Status,
	}), nil
}

func (s *Service) GetGame(_ context.Context, req *connect.Request[tictactoev1.GetGameRequest]) (*connect.Response[tictactoev1.GetGameResponse], error) {
	m, ok := s.matches.Get(req.Msg.GameId)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, ErrGameNotFound)
	}
	return connect.NewResponse(&tictactoev1.GetGameResponse{
		Status:     m.Status,
		WorkerAddr: m.WorkerAddr,
		PlayerX:    m.Creator,
		PlayerO:    m.Opponent,
	}), nil
}

func (s *Service) GetStats(_ context.Context, req *connect.Request[tictactoev1.GetStatsRequest]) (*connect.Response[tictactoev1.GetStatsResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}
	v := s.stats.Get(req.Msg.UserId)
	return connect.NewResponse(&tictactoev1.GetStatsResponse{
		UserId: req.Msg.UserId,
		Wins:   v.Wins,
		Losses: v.Losses,
		Draws:  v.Draws,
	}), nil
}

func (s *Service) HandleAssigned(_ context.Context, body []byte) error {
	var msg rabbitmq.GameAssigned
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}
	s.matches.CompleteJoin(msg.GameID, s.browserAddr(msg.GameID, msg.WorkerAddr))
	return nil
}

func (s *Service) HandleResult(_ context.Context, body []byte) error {
	var msg rabbitmq.GameResult
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}
	s.matches.Finish(msg.GameID)
	if msg.Draw {
		s.stats.AddDraw(msg.PlayerX)
		s.stats.AddDraw(msg.PlayerO)
		return nil
	}
	loser := msg.PlayerX
	if loser == msg.WinnerID {
		loser = msg.PlayerO
	}
	s.stats.AddWin(msg.WinnerID)
	s.stats.AddLoss(loser)
	return nil
}

// browserAddr is the worker route the client is handed by GetGame. With a
// routing secret (the gateway is in front) it is a signed capability; gatewayURL
// is usually empty, making it a relative "/w/<token>" the client resolves against
// its own origin — so it works from any host and never crosses origins. Without a
// secret there is no gateway, so the worker is addressed directly.
func (s *Service) browserAddr(gameID, internalAddr string) string {
	if len(s.routeSecret) == 0 {
		return internalAddr
	}
	token, err := routing.Sign(s.routeSecret, gameID, internalAddr, s.routeTTL)
	if err != nil {
		return internalAddr
	}
	return s.gatewayURL + "/w/" + token
}

func joinError(err error) error {
	switch {
	case errors.Is(err, ErrGameNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, ErrCannotJoinOwn):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
}

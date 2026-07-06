// Package config is the single source of truth for runtime configuration. Every
// environment variable and its default is declared once, as a struct tag on the
// Settings field that reads it, so there is one place to discover what can be
// tuned and by which role. Settings are read once at startup (Load) and are not
// expected to change afterwards.
package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/codingconcepts/env"
)

// Settings is the full configuration for a process, loaded once from the
// environment via struct tags. Fields are grouped by the role that reads them.
type Settings struct {
	// Shared by every role.
	AMQPURL string `env:"AMQP_URL" default:"amqp://guest:guest@localhost:5672/"` // RabbitMQ connection string.

	// Lobby: the authority (matchmaking + stats).
	BoardSize       int           `env:"BOARD_SIZE" default:"3"`            // default N when a game omits it.
	WinLength       int           `env:"WIN_LENGTH" default:"3"`            // default K when a game omits it.
	LobbyListenAddr string        `env:"LOBBY_LISTEN_ADDR" default:":8080"` //
	GatewayURL      string        `env:"GATEWAY_URL"`                       // public origin clients reach workers through ("" = relative).
	RoutingSecret   string        `env:"ROUTING_SECRET"`                    // HMAC key for worker-route capabilities ("" = unsigned, dev only).
	RoutingTTL      time.Duration `env:"ROUTING_TTL" default:"1h"`          // how long a worker-route token stays valid.
	ReapInterval    time.Duration `env:"REAP_INTERVAL" default:"1m"`        // how often the lobby scans for stale games.
	GameTTL         time.Duration `env:"GAME_TTL" default:"30m"`            // idle time after which a lobby game is abandoned.

	// Worker: holds live games.
	WorkerListenAddr    string        `env:"WORKER_LISTEN_ADDR" default:":8080"`   //
	WorkerAdvertiseAddr string        `env:"WORKER_ADVERTISE_ADDR"`                // address peers reach this worker at (defaults to its hostname).
	WorkerDNSSuffix     string        `env:"WORKER_DNS_SUFFIX"`                    // appended to the hostname for the advertise default (e.g. ".workers" on K8s).
	WorkerMaxGames      int           `env:"WORKER_MAX_GAMES" default:"200"`       // most games one worker will host at once.
	WorkerReapInterval  time.Duration `env:"WORKER_REAP_INTERVAL" default:"30s"`   // how often the worker scans for evictable games.
	FinishedLinger      time.Duration `env:"WORKER_FINISHED_LINGER" default:"1m"`  // keep a finished game this long so both players see the result.
	AbandonedAfter      time.Duration `env:"WORKER_ABANDONED_AFTER" default:"30m"` // drop a game with no move for this long.

	// Gateway: the stateless edge.
	GatewayListenAddr string `env:"GATEWAY_LISTEN_ADDR" default:":8080"`        //
	LobbyAddr         string `env:"LOBBY_ADDR" default:"http://localhost:8080"` // internal address of the lobby to proxy to.

	// DefaultGame is derived from BoardSize/WinLength in Load (no env tag, so the
	// loader skips it); it is the validated rules a new game falls back to.
	DefaultGame Game
}

// Load reads the environment once (applying each field's default) and returns the
// resolved settings. It fails only if the default game rules are invalid.
func Load() (Settings, error) {
	var s Settings
	if err := env.Set(&s); err != nil {
		return Settings{}, fmt.Errorf("load configuration: %w", err)
	}

	// The advertise address defaults to this host's own name (see below).
	if s.WorkerAdvertiseAddr == "" {
		s.WorkerAdvertiseAddr = defaultAdvertiseAddr(s.WorkerListenAddr, s.WorkerDNSSuffix)
	}

	game, err := NewGame(WithBoardSize(s.BoardSize), WithWinLength(s.WinLength))
	if err != nil {
		return Settings{}, fmt.Errorf("invalid default game configuration: %w", err)
	}
	s.DefaultGame = game
	return s, nil
}

// defaultAdvertiseAddr builds the address others reach this worker at from its
// DNS-resolvable hostname (never a self-guessed IP). Under Docker --scale each
// replica's hostname is its unique container id; under a Kubernetes StatefulSet it
// is the stable pod name, and dnsSuffix names the headless service (e.g.
// ".workers") so the record survives IP changes.
func defaultAdvertiseAddr(listenAddr, dnsSuffix string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		port = "8080"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host+dnsSuffix, port)
}

// --- game rules (board size N, win length K) ---

type Game struct {
	BoardSize int
	WinLength int
}

type Option func(*Game)

func WithBoardSize(n int) Option { return func(g *Game) { g.BoardSize = n } }
func WithWinLength(k int) Option { return func(g *Game) { g.WinLength = k } }

// NewGame builds and validates a rules configuration. It is reused for both the
// server defaults and each per-game request, so validation lives in one place.
func NewGame(opts ...Option) (Game, error) {
	g := Game{BoardSize: 3, WinLength: 3}
	for _, opt := range opts {
		opt(&g)
	}
	switch {
	case g.BoardSize < 3:
		return Game{}, fmt.Errorf("board size must be >= 3, got %d", g.BoardSize)
	case g.WinLength < 1:
		return Game{}, fmt.Errorf("win length must be >= 1, got %d", g.WinLength)
	case g.WinLength > g.BoardSize:
		return Game{}, fmt.Errorf("win length %d must not exceed board size %d", g.WinLength, g.BoardSize)
	}
	return g, nil
}

// Package gateway is the stateless edge: it proxies lobby calls and routes each
// move to the worker named in the signed /w/<token> path.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/vk496/tictactoe_project/api/gen/openapi"
	"github.com/vk496/tictactoe_project/internal/config"
	"github.com/vk496/tictactoe_project/internal/httpx"
	"github.com/vk496/tictactoe_project/internal/routing"
)

const workerPrefix = "/w/"

func Run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lobbyURL, err := url.Parse(cfg.LobbyAddr)
	if err != nil {
		return fmt.Errorf("invalid LOBBY_ADDR: %w", err)
	}
	secret := []byte(cfg.RoutingSecret)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, workerPrefix):
			proxyWorker(w, r, secret)
		case strings.HasPrefix(r.URL.Path, "/tictactoe.v1.Lobby/"):
			proxy(w, r, lobbyURL, r.URL.Path)
		case r.URL.Path == "/openapi.yaml":
			serveOpenAPI(w)
		default:
			http.NotFound(w, r)
		}
	})

	logger.Info("gateway starting", "lobby", cfg.LobbyAddr)
	return httpx.Serve(ctx, httpx.NewServer(cfg.GatewayListenAddr, handler), logger)
}

func proxyWorker(w http.ResponseWriter, r *http.Request, secret []byte) {
	rest := strings.TrimPrefix(r.URL.Path, workerPrefix)
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		http.Error(w, "missing worker route", http.StatusBadRequest)
		return
	}
	token, path := rest[:slash], rest[slash:]
	if !strings.HasPrefix(path, "/tictactoe.v1.Worker/") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	host, err := resolveWorker(secret, token)
	if err != nil {
		http.Error(w, "invalid route token", http.StatusForbidden)
		return
	}

	rp := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: host})
	rp.ErrorHandler = workerUnreachable
	r.URL.Path = path
	rp.ServeHTTP(w, r)
}

// workerUnreachable is returned when the worker hosting a game can no longer be
// reached (its hostname does not resolve, or it refuses the connection). The
// game's in-memory state is gone, so the client is told to start a new one.
func workerUnreachable(w http.ResponseWriter, _ *http.Request, _ error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	_, _ = w.Write([]byte(`{"message":"game is no longer available; please start a new game"}`))
}

// resolveWorker turns the routing segment into a worker host. With a secret it
// is a signed JWT capability the gateway verifies; without one it falls back to
// treating the segment as a raw authority (development only).
func resolveWorker(secret []byte, token string) (string, error) {
	if len(secret) == 0 {
		return token, nil
	}
	workerAddr, _, err := routing.Verify(secret, token)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(workerAddr)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func proxy(w http.ResponseWriter, r *http.Request, target *url.URL, path string) {
	rp := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = path
	rp.ServeHTTP(w, r)
}

// serveOpenAPI returns the OpenAPI spec generated from the protos, so the API is
// documented at the same origin it is served from.
func serveOpenAPI(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openapi.Spec)
}

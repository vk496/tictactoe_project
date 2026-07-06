// Package httpx builds the HTTP server the three roles share: one endpoint that
// speaks HTTP/1.1 and cleartext HTTP/2 — so the Connect protocol, gRPC-Web and
// gRPC all work — plus graceful shutdown. No CORS: everything is served behind a
// single origin (the Traefik edge), so browser requests are same-origin.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

func NewServer(addr string, handler http.Handler) *http.Server {
	// Go 1.24+ serves cleartext HTTP/2 (h2c) natively via Protocols.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func Serve(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		logger.Info("shutting down")
		return srv.Shutdown(shutCtx)
	}
}

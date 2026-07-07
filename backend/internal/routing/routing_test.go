package routing_test

import (
	"testing"
	"time"

	"github.com/vk496/tictactoe_project/internal/routing"
)

const (
	secret     = "shared-secret"
	workerAddr = "http://worker-3:8080"
	gameID     = "game-1"
)

// ADR 7: a token signed with the secret round-trips back to the same worker
// address and game.
func TestTokenRoundTrip(t *testing.T) {
	token, err := routing.Sign([]byte(secret), gameID, workerAddr, time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	gotAddr, gotGame, err := routing.Verify([]byte(secret), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotAddr != workerAddr || gotGame != gameID {
		t.Fatalf("round-trip = (%q, %q); want (%q, %q)", gotAddr, gotGame, workerAddr, gameID)
	}
}

// ADR 7: a forged, tampered, or expired token is rejected — the gateway can never
// be pointed at an arbitrary host.
func TestTokenRejectsForgery(t *testing.T) {
	valid, _ := routing.Sign([]byte(secret), gameID, workerAddr, time.Hour)
	expired, _ := routing.Sign([]byte(secret), gameID, workerAddr, -time.Hour)

	tests := []struct {
		name   string
		secret string
		token  string
	}{
		{"wrong secret", "other-secret", valid},
		{"tampered token", secret, valid + "x"},
		{"expired token", secret, expired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := routing.Verify([]byte(tt.secret), tt.token); err == nil {
				t.Fatal("expected verification to fail")
			}
		})
	}
}

package routing_test

import (
	"testing"
	"time"

	"github.com/vk496/tictactoe_project/internal/routing"
)

// ADR 7: a route token round-trips only with the right secret.
func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("shared-secret")
	tok, err := routing.Sign(secret, "game-1", "http://worker-3:8080", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	addr, gameID, err := routing.Verify(secret, tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if addr != "http://worker-3:8080" || gameID != "game-1" {
		t.Fatalf("round-trip mismatch: addr=%q game=%q", addr, gameID)
	}
}

// ADR 7: a forged, tampered or expired token is rejected — the gateway can never
// be pointed at an arbitrary host.
func TestTokenRejectsForgery(t *testing.T) {
	secret := []byte("shared-secret")
	tok, _ := routing.Sign(secret, "g", "http://w:8080", time.Hour)

	if _, _, err := routing.Verify([]byte("wrong-secret"), tok); err == nil {
		t.Fatal("wrong secret should not verify")
	}
	if _, _, err := routing.Verify(secret, tok+"x"); err == nil {
		t.Fatal("tampered token should not verify")
	}
	expired, _ := routing.Sign(secret, "g", "http://w:8080", -time.Hour)
	if _, _, err := routing.Verify(secret, expired); err == nil {
		t.Fatal("expired token should not verify")
	}
}

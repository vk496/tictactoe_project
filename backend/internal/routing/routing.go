package routing

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type claims struct {
	WorkerAddr string `json:"waddr"`
	jwt.RegisteredClaims
}

// Sign returns a signed capability that authorises routing to workerAddr for the
// given game. The token is opaque to the client and cannot be tampered with.
func Sign(secret []byte, gameID, workerAddr string, ttl time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		WorkerAddr: workerAddr,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   gameID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return token.SignedString(secret)
}

// Verify checks the signature and expiry and returns the worker address the
// caller is allowed to reach.
func Verify(secret []byte, token string) (workerAddr, gameID string, err error) {
	var c claims
	_, err = jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return "", "", err
	}
	return c.WorkerAddr, c.Subject, nil
}

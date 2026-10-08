package entities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const KeyGrace = 10 * time.Minute

type Credentials struct {
	ServiceID             string
	Environment           string
	TokenCiphertext       []byte
	TokenNonce            []byte
	TokenHash             string
	KeyCiphertext         []byte
	KeyNonce              []byte
	PreviousKeyCiphertext []byte
	PreviousKeyNonce      []byte
	KeyID                 string
	RotatedAt             *time.Time
}

func NewSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (c Credentials) PreviousKeyValid(now time.Time) bool {
	return c.RotatedAt != nil && len(c.PreviousKeyCiphertext) > 0 && now.Sub(*c.RotatedAt) < KeyGrace
}

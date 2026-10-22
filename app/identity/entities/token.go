package entities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/klzn-labs/kalzone/pkg/actor"
)

func NewToken() (string, error) {
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

type Authenticated struct {
	User      User
	Teams     []string
	CSRFToken string
}

func (a Authenticated) Actor() actor.Actor {
	return actor.Actor{UserID: a.User.ID, Login: a.User.Login, Admin: a.User.Admin, Guest: a.User.Guest(), Teams: a.Teams}
}

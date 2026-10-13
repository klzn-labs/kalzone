package entities

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/scope"
)

const (
	TokenPrefix     = "plat_"
	tokenBodyLength = 43
	displayLength   = 8
)

var (
	ErrTokenInvalid = apperror.New(apperror.CodeTokenInvalid, "this access token is not valid")
	ErrTokenExpired = apperror.New(apperror.CodeTokenExpired, "this access token has expired, create a new one")
	ErrTokenRevoked = apperror.New(apperror.CodeTokenRevoked, "this access token was revoked")
)

type AccessToken struct {
	ID               string
	OwnerID          string
	Name             string
	Hash             string
	Prefix           string
	Scopes           []scope.Scope
	TeamIDs          []string
	TeamSlugs        []string
	Bypass           bool
	CreatedAt        time.Time
	CreatedByTokenID string
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	LastUsedIP       string
	RevokedAt        *time.Time
}

type TokenSecret struct {
	Raw    string
	Prefix string
	Hash   string
}

func NewTokenSecret() (TokenSecret, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return TokenSecret{}, err
	}
	body := new(big.Int).SetBytes(random).Text(62)
	body = strings.Repeat("0", tokenBodyLength-len(body)) + body
	raw := TokenPrefix + body
	return TokenSecret{Raw: raw, Prefix: body[:displayLength], Hash: HashToken(raw)}, nil
}

func LooksLikeToken(raw string) bool {
	return strings.HasPrefix(raw, TokenPrefix) && len(raw) == len(TokenPrefix)+tokenBodyLength
}

func (t AccessToken) Check(now time.Time) error {
	if t.RevokedAt != nil {
		return ErrTokenRevoked
	}
	if !now.Before(t.ExpiresAt) {
		return ErrTokenExpired
	}
	return nil
}

type TokenAuthenticated struct {
	User  User
	Token AccessToken
}

func (t TokenAuthenticated) Actor() actor.Actor {
	return actor.Actor{UserID: t.User.ID, Login: t.User.Login, Admin: t.User.Admin, Token: actor.Token{
		ID: t.Token.ID, Name: t.Token.Name, Prefix: t.Token.Prefix, Scopes: t.Token.Scopes,
		TeamIDs: t.Token.TeamIDs, Bypass: t.Token.Bypass, ExpiresAt: t.Token.ExpiresAt,
	}}
}

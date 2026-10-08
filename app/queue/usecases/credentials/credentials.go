package credentials

import (
	"context"
	"errors"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/clock"
)

type Store interface {
	Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error)
	CreateCredentials(ctx context.Context, c entities.Credentials) (bool, error)
	SaveCredentials(ctx context.Context, c entities.Credentials) error
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
	Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error)
}

type Opened struct {
	Token       string
	SigningKey  string
	PreviousKey string
}

type CredentialsUsecase struct {
	store  Store
	sealer Sealer
}

func NewCredentialsUsecase(store Store, sealer Sealer) CredentialsUsecase {
	return CredentialsUsecase{store: store, sealer: sealer}
}

func (u CredentialsUsecase) Ensure(ctx context.Context, serviceID, environment string) (Opened, error) {
	c, found, err := u.store.Credentials(ctx, serviceID, environment)
	if err != nil {
		return Opened{}, err
	}
	if found {
		return u.Open(c)
	}
	return u.create(ctx, serviceID, environment)
}

func (u CredentialsUsecase) create(ctx context.Context, serviceID, environment string) (Opened, error) {
	c, opened, err := u.issue(entities.Credentials{ServiceID: serviceID, Environment: environment}, "")
	if err != nil {
		return Opened{}, err
	}
	created, err := u.store.CreateCredentials(ctx, c)
	if err != nil || created {
		return opened, err
	}
	winner, _, err := u.store.Credentials(ctx, serviceID, environment)
	if err != nil {
		return Opened{}, err
	}
	return u.Open(winner)
}

func (u CredentialsUsecase) Rotate(ctx context.Context, serviceID, environment string) (Opened, error) {
	current, err := u.Ensure(ctx, serviceID, environment)
	if err != nil {
		return Opened{}, err
	}
	c, opened, err := u.issue(entities.Credentials{ServiceID: serviceID, Environment: environment}, current.SigningKey)
	if err != nil {
		return Opened{}, err
	}
	return opened, u.store.SaveCredentials(ctx, c)
}

func (u CredentialsUsecase) Open(c entities.Credentials) (Opened, error) {
	token, err := u.sealer.Open(c.TokenCiphertext, c.TokenNonce, aad(c, "token"), c.KeyID)
	if err != nil {
		return Opened{}, err
	}
	key, err := u.sealer.Open(c.KeyCiphertext, c.KeyNonce, aad(c, "key"), c.KeyID)
	if err != nil {
		return Opened{}, err
	}
	previous, err := u.previous(c)
	return Opened{Token: string(token), SigningKey: string(key), PreviousKey: previous}, err
}

func (u CredentialsUsecase) previous(c entities.Credentials) (string, error) {
	if !c.PreviousKeyValid(clock.Now().UTC()) {
		return "", nil
	}
	raw, err := u.sealer.Open(c.PreviousKeyCiphertext, c.PreviousKeyNonce, aad(c, "previous"), c.KeyID)
	return string(raw), err
}

func aad(c entities.Credentials, part string) []byte {
	return []byte("queue|" + c.ServiceID + "|" + c.Environment + "|" + part)
}

func (u CredentialsUsecase) issue(c entities.Credentials, previousKey string) (entities.Credentials, Opened, error) {
	token, tokenErr := entities.NewSecret()
	key, keyErr := entities.NewSecret()
	if err := errors.Join(tokenErr, keyErr); err != nil {
		return entities.Credentials{}, Opened{}, err
	}
	c.TokenHash = entities.HashToken(token)
	if err := u.sealAll(&c, token, key, previousKey); err != nil {
		return entities.Credentials{}, Opened{}, err
	}
	return c, Opened{Token: token, SigningKey: key, PreviousKey: previousKey}, nil
}

func (u CredentialsUsecase) sealAll(c *entities.Credentials, token, key, previousKey string) error {
	var err error
	if c.TokenCiphertext, c.TokenNonce, c.KeyID, err = u.sealer.Seal([]byte(token), aad(*c, "token")); err != nil {
		return err
	}
	if c.KeyCiphertext, c.KeyNonce, _, err = u.sealer.Seal([]byte(key), aad(*c, "key")); err != nil {
		return err
	}
	if previousKey == "" {
		return nil
	}
	now := clock.Now().UTC()
	c.RotatedAt = &now
	c.PreviousKeyCiphertext, c.PreviousKeyNonce, _, err = u.sealer.Seal([]byte(previousKey), aad(*c, "previous"))
	return err
}

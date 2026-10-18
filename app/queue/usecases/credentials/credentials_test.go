package credentials_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type store struct {
	saved map[string]entities.Credentials
}

func (s *store) Credentials(_ context.Context, serviceID, env string) (entities.Credentials, bool, error) {
	c, ok := s.saved[serviceID+env]
	return c, ok, nil
}

func (s *store) CreateCredentials(_ context.Context, c entities.Credentials) (bool, error) {
	if _, taken := s.saved[c.ServiceID+c.Environment]; taken {
		return false, nil
	}
	s.saved[c.ServiceID+c.Environment] = c
	return true, nil
}

func (s *store) SaveCredentials(_ context.Context, c entities.Credentials) error {
	s.saved[c.ServiceID+c.Environment] = c
	return nil
}

func newUsecase(t *testing.T) (credentials.CredentialsUsecase, *store) {
	t.Helper()
	box, err := secretbox.NewWithKey("k1", []byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	s := &store{saved: map[string]entities.Credentials{}}
	return credentials.NewCredentialsUsecase(s, box), s
}

func Test_Ensure_GeneratesOnceAndSealsAtRest(t *testing.T) {
	u, s := newUsecase(t)

	first, err := u.Ensure(ctx, "svc", "prod")
	second, _ := u.Ensure(ctx, "svc", "prod")

	require.NoError(t, err)
	require.Len(t, first.Token, 64)
	require.Len(t, first.SigningKey, 64)
	require.Equal(t, first, second)
	stored := s.saved["svcprod"]
	require.Equal(t, entities.HashToken(first.Token), stored.TokenHash)
	require.NotContains(t, string(stored.TokenCiphertext), first.Token)
}

func Test_Rotate_KeepsThePreviousKeyForTheGracePeriod(t *testing.T) {
	u, _ := newUsecase(t)
	before, _ := u.Ensure(ctx, "svc", "prod")

	after, err := u.Rotate(ctx, "svc", "prod")

	require.NoError(t, err)
	require.NotEqual(t, before.Token, after.Token)
	require.NotEqual(t, before.SigningKey, after.SigningKey)
	require.Equal(t, before.SigningKey, after.PreviousKey)
	clock.WithTime(time.Now().Add(entities.KeyGrace + time.Minute))
	t.Cleanup(func() { clock.Clock = nil })
	expired, _ := u.Ensure(ctx, "svc", "prod")
	require.Empty(t, expired.PreviousKey)
}

type racingStore struct {
	winner entities.Credentials
	reads  int
}

func (s *racingStore) Credentials(context.Context, string, string) (entities.Credentials, bool, error) {
	s.reads++
	return s.winner, s.reads > 1, nil
}

func (s *racingStore) CreateCredentials(context.Context, entities.Credentials) (bool, error) {
	return false, nil
}

func (s *racingStore) SaveCredentials(context.Context, entities.Credentials) error {
	return nil
}

func Test_Ensure_ReturnsTheCredentialsThatWonARace(t *testing.T) {
	winnerUsecase, winnerStore := newUsecase(t)
	winner, _ := winnerUsecase.Ensure(ctx, "svc", "prod")
	box, err := secretbox.NewWithKey("k1", []byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	racing := &racingStore{winner: winnerStore.saved["svcprod"]}

	got, err := credentials.NewCredentialsUsecase(racing, box).Ensure(ctx, "svc", "prod")

	require.NoError(t, err)
	require.Equal(t, winner.Token, got.Token)
	require.Equal(t, winner.SigningKey, got.SigningKey)
}

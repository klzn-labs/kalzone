package signing_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/cron/repositories/signing"
	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/stretchr/testify/require"
)

type ensured struct{ asked []string }

func (e *ensured) Ensure(_ context.Context, serviceID, environment string) (credentials.Opened, error) {
	e.asked = append(e.asked, serviceID+"/"+environment)
	return credentials.Opened{Token: "token", SigningKey: "key", PreviousKey: "old"}, nil
}

func Test_Keys_AreTheServicesQueueSigningKeys(t *testing.T) {
	creds := &ensured{}
	var _ executor.Credentials = signing.NewKeys(creds)

	keys, err := signing.NewKeys(creds).Keys(context.Background(), "s1", "dev")

	require.NoError(t, err)
	require.Equal(t, executor.Keys{SigningKey: "key", PreviousKey: "old"}, keys)
	require.Equal(t, []string{"s1/dev"}, creds.asked)
}

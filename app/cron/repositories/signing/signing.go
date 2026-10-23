package signing

import (
	"context"

	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
)

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type Keys struct {
	credentials Credentials
}

func NewKeys(credentials Credentials) Keys {
	return Keys{credentials: credentials}
}

func (k Keys) Keys(ctx context.Context, serviceID, environment string) (executor.Keys, error) {
	opened, err := k.credentials.Ensure(ctx, serviceID, environment)
	return executor.Keys{SigningKey: opened.SigningKey, PreviousKey: opened.PreviousKey}, err
}

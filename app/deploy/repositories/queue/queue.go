package queue

import (
	"context"

	"github.com/adrianozp/platform/app/queue/usecases/credentials"
)

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type Access struct {
	creds      Credentials
	gatewayURL string
}

func NewAccess(creds Credentials, gatewayURL string) Access {
	return Access{creds: creds, gatewayURL: gatewayURL}
}

func (a Access) QueueAccess(ctx context.Context, serviceID, environment string) (map[string]string, map[string]string, error) {
	opened, err := a.creds.Ensure(ctx, serviceID, environment)
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{"PLATFORM_QUEUE_URL": a.gatewayURL}
	secrets := map[string]string{"PLATFORM_QUEUE_TOKEN": opened.Token, "PLATFORM_QUEUE_SIGNING_KEY": opened.SigningKey}
	return env, secrets, nil
}

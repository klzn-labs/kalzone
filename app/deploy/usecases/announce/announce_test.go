package announce_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/deploy/usecases/announce"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

type notifier struct{ sent []string }

func (n *notifier) Notify(text string) { n.sent = append(n.sent, text) }

func deployed(action, environment string) events.Event {
	return events.Event{
		Action: action, TargetKind: events.TargetDeployment, TargetName: "api", Environment: environment,
		Details: map[string]any{"deployment": 17, "release": 42, "trigger": "promote"},
	}
}

func Test_Handle_AnnouncesASuccessfulProdDeploy(t *testing.T) {
	n := &notifier{}
	usecase := announce.NewAnnounceUsecase(config.Config{BaseURL: "https://platform.kalz.one/"}, n)

	require.NoError(t, usecase.Handle(context.Background(), deployed("deploy.succeeded", "prod")))

	require.Equal(t, []string{"api r42 is live on prod (deployment #17, promote): https://platform.kalz.one/services/api/env/prod"}, n.sent)
}

func Test_Handle_StaysQuietAboutEverythingElse(t *testing.T) {
	n := &notifier{}
	usecase := announce.NewAnnounceUsecase(config.Config{BaseURL: "https://platform.kalz.one"}, n)

	for _, e := range []events.Event{deployed("deploy.succeeded", "dev"), deployed("deploy.failed", "prod"), deployed("deploy.requested", "prod")} {
		require.NoError(t, usecase.Handle(context.Background(), e))
	}

	require.Empty(t, n.sent)
}

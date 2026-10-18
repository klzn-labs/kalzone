package gateway_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/queue/usecases/gateway"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type source struct {
	queues   []entities.Queue
	commands []entities.Command
	creds    []entities.Credentials
	done     []string
}

func (s *source) All(context.Context) ([]entities.Queue, error)               { return s.queues, nil }
func (s *source) PendingCommands(context.Context) ([]entities.Command, error) { return s.commands, nil }
func (s *source) AllCredentials(context.Context) ([]entities.Credentials, error) {
	return s.creds, nil
}
func (s *source) FinishCommand(_ context.Context, id string, _ time.Time) error {
	s.done = append(s.done, id)
	return nil
}

type services struct{}

func (services) FindServiceByID(_ context.Context, id string) (queues.ServiceRef, error) {
	if id == "s-worker" || id == "s-broken" || id == "s-importer" {
		return queues.ServiceRef{ID: id, Slug: id}, nil
	}
	return queues.ServiceRef{}, apperror.NotFound("service not found")
}

type ports struct{}

func (ports) Port(_ context.Context, serviceID, _ string) (int, error) {
	if serviceID == "s-broken" {
		return 0, errors.New("config could not be read")
	}
	return 8081, nil
}

type creds struct{}

func (creds) Ensure(_ context.Context, serviceID, _ string) (credentials.Opened, error) {
	return credentials.Opened{SigningKey: "key-" + serviceID}, nil
}

func placeImport(consumer string) entities.Queue {
	return entities.Queue{
		ID: "q-" + consumer, Name: "place-import", Environment: "prod", ConsumerID: consumer, Version: 3,
		Settings: entities.DefaultSettings("/hooks/place-import"), Publishers: []string{consumer, "s-importer", "s-new"},
	}
}

func Test_Desired_BuildsTargetsKeysAndPublisherHashes(t *testing.T) {
	src := &source{
		queues: []entities.Queue{placeImport("s-worker")},
		creds: []entities.Credentials{
			{ServiceID: "s-importer", Environment: "prod", TokenHash: "h-importer"},
			{ServiceID: "s-importer", Environment: "dev", TokenHash: "h-importer-dev"},
		},
		commands: []entities.Command{{ID: "c1", Kind: entities.CommandReplayAll}},
	}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Queues, 1)
	q := desired.Queues[0]
	require.Equal(t, "http://s-worker-prod:8081/hooks/place-import", q.Target)
	require.Equal(t, "key-s-worker", q.SigningKey)
	require.Equal(t, []string{"h-importer"}, q.PublisherHashes)
	require.Equal(t, "c1", desired.Commands[0].ID)
}

func Test_Desired_SkipsDeletedServices(t *testing.T) {
	src := &source{queues: []entities.Queue{placeImport("s-gone"), placeImport("s-worker")}}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Queues, 1)
	require.Equal(t, "q-s-worker", desired.Queues[0].ID)
}

func Test_Desired_DropsTheTokensOfDeletedPublishers(t *testing.T) {
	src := &source{
		queues: []entities.Queue{placeImport("s-worker")},
		creds: []entities.Credentials{
			{ServiceID: "s-importer", Environment: "prod", TokenHash: "h-importer"},
			{ServiceID: "s-new", Environment: "prod", TokenHash: "h-deleted"},
		},
	}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Equal(t, []string{"h-importer"}, desired.Queues[0].PublisherHashes)
}

func Test_Desired_LeavesOutAQueueThatFailsAndKeepsTheRest(t *testing.T) {
	src := &source{queues: []entities.Queue{placeImport("s-broken"), placeImport("s-worker")}}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Queues, 1)
	require.Equal(t, "q-s-worker", desired.Queues[0].ID)
}

func Test_Report_AndDone(t *testing.T) {
	src := &source{}
	stats := gateway.NewStatsStore()
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, stats)

	require.NoError(t, u.Report(ctx, []entities.Stats{{QueueID: "q1", Depth: 4}}))
	require.NoError(t, u.Done(ctx, "c9"))

	latest, ok := stats.Latest("q1")
	require.True(t, ok)
	require.EqualValues(t, 4, latest.Depth)
	require.False(t, latest.ReportedAt.IsZero())
	require.Equal(t, []string{"c9"}, src.done)
}

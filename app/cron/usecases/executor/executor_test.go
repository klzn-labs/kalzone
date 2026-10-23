package executor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type source struct {
	crons    []entities.Cron
	commands []entities.Command
	finished []string
	recorded []entities.Run
}

func (s *source) All(context.Context) ([]entities.Cron, error) { return s.crons, nil }

func (s *source) PendingCommands(context.Context) ([]entities.Command, error) { return s.commands, nil }

func (s *source) FinishCommand(_ context.Context, id string, _ time.Time) error {
	s.finished = append(s.finished, id)
	return nil
}

func (s *source) RecordRun(_ context.Context, run entities.Run) error {
	s.recorded = append(s.recorded, run)
	return nil
}

type services map[string]crons.ServiceRef

func (s services) FindServiceByID(_ context.Context, id string) (crons.ServiceRef, error) {
	ref, ok := s[id]
	if !ok {
		return crons.ServiceRef{}, apperror.NotFound("service not found")
	}
	return ref, nil
}

type ports struct{ broken string }

func (p ports) Port(_ context.Context, serviceID, _ string) (int, error) {
	if serviceID == p.broken {
		return 0, errors.New("no config")
	}
	return 8080, nil
}

type keys struct{}

func (keys) Keys(_ context.Context, serviceID, environment string) (executor.Keys, error) {
	return executor.Keys{SigningKey: "key-" + serviceID + "-" + environment, PreviousKey: "old"}, nil
}

var catalog = services{
	"s-worker":  {ID: "s-worker", Slug: "worker"},
	"s-leaving": {ID: "s-leaving", Slug: "leaving", Decommissioning: true},
	"s-broken":  {ID: "s-broken", Slug: "broken"},
}

func cron(id, serviceID string) entities.Cron {
	return entities.Cron{ID: id, Name: "job-" + id, Environment: "dev", ServiceID: serviceID, Schedule: "@hourly", Path: "/hooks/" + id, Timeout: time.Minute}
}

func Test_Desired_TargetsTheServiceWithItsSigningKey(t *testing.T) {
	withPayload := cron("c1", "s-worker")
	withPayload.Payload, withPayload.Paused = []byte(`{"a":1}`), true
	src := &source{crons: []entities.Cron{withPayload, cron("c2", "s-worker")}}

	desired, err := executor.NewExecutorUsecase(src, catalog, ports{}, keys{}).Desired(ctx)

	require.NoError(t, err)
	require.Equal(t, executor.DesiredCron{
		ID: "c1", Name: "job-c1", Environment: "dev", Service: "worker", Schedule: "@hourly",
		Target: "http://worker-dev:8080/hooks/c1", Payload: []byte(`{"a":1}`), Timeout: time.Minute, Paused: true,
		SigningKey: "key-s-worker-dev", PreviousKey: "old",
	}, desired.Crons[0])
	require.Equal(t, "{}", string(desired.Crons[1].Payload))
}

func Test_Desired_LeavesOutCronsOfMissingDeletedOrBrokenServices(t *testing.T) {
	src := &source{crons: []entities.Cron{cron("c1", "s-gone"), cron("c2", "s-leaving"), cron("c3", "s-broken"), cron("c4", "s-worker")}}

	desired, err := executor.NewExecutorUsecase(src, catalog, ports{broken: "s-broken"}, keys{}).Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Crons, 1)
	require.Equal(t, "c4", desired.Crons[0].ID)
}

func Test_Desired_CarriesOnlyCommandsOfDesiredCrons(t *testing.T) {
	src := &source{
		crons:    []entities.Cron{cron("c1", "s-worker"), cron("c2", "s-leaving")},
		commands: []entities.Command{{ID: "k1", CronID: "c1", Kind: entities.CommandRun}, {ID: "k2", CronID: "c2", Kind: entities.CommandRun}},
	}

	desired, err := executor.NewExecutorUsecase(src, catalog, ports{}, keys{}).Desired(ctx)

	require.NoError(t, err)
	require.Equal(t, []entities.Command{src.commands[0]}, desired.Commands)
}

func Test_RecordAndDone_ReachTheStore(t *testing.T) {
	src := &source{}
	u := executor.NewExecutorUsecase(src, catalog, ports{}, keys{})
	runs := []entities.Run{{ID: "r1", CronID: "c1"}, {ID: "r2", CronID: "c1"}}

	require.NoError(t, u.Record(ctx, runs))
	require.NoError(t, u.Done(ctx, "k1"))

	require.Equal(t, runs, src.recorded)
	require.Equal(t, []string{"k1"}, src.finished)
}

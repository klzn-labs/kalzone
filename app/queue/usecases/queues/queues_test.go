package queues_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_Create_FillsDefaultsOwnerAndConsumerAsPublisher(t *testing.T) {
	f := newFixture(t, false)

	q, err := f.usecase.Create(ctx, ana, draft())

	require.NoError(t, err)
	require.Equal(t, "t-plateful", q.TeamID)
	require.Equal(t, "s-worker", q.ConsumerID)
	require.Equal(t, entities.DefaultSettings("/hooks/place-import"), q.Settings)
	require.ElementsMatch(t, []string{"s-worker", "s-importer"}, q.Publishers)
	require.Equal(t, 1, q.Version)
	require.Equal(t, []string{"queue.created"}, f.events.actions)
}

func Test_Create_RefusesBadInputAndOutsiders(t *testing.T) {
	f := newFixture(t, false)
	denied := newFixture(t, true)
	badName, badEnv, badConsumer, badPublisher, badSettings := draft(), draft(), draft(), draft(), draft()
	badName.Name = "Bad"
	badEnv.Environment = "staging"
	badConsumer.Consumer = "ghost"
	badPublisher.Publishers = []string{"ghost"}
	badSettings.Settings.MaxInFlight = 500

	_, e1 := f.usecase.Create(ctx, ana, badName)
	_, e2 := f.usecase.Create(ctx, ana, badEnv)
	_, e3 := f.usecase.Create(ctx, ana, badConsumer)
	_, e4 := f.usecase.Create(ctx, ana, badPublisher)
	_, e5 := f.usecase.Create(ctx, ana, badSettings)
	_, e6 := denied.usecase.Create(ctx, ana, draft())

	require.ErrorIs(t, e1, apperror.Invalid(""))
	require.ErrorIs(t, e2, apperror.NotFound(""))
	require.ErrorIs(t, e3, apperror.NotFound(""))
	require.ErrorIs(t, e4, apperror.NotFound(""))
	require.ErrorIs(t, e5, apperror.Invalid(""))
	require.ErrorIs(t, e6, apperror.Forbidden(""))
	require.Empty(t, f.store.queues)
}

func Test_Update_BumpsTheVersion(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	settings := created.Settings
	settings.MaxAttempts = 9

	q, err := f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &settings, Message: "more retries", BaseVersion: 1})

	require.NoError(t, err)
	require.Equal(t, 2, q.Version)
	require.Equal(t, 9, q.Settings.MaxAttempts)
	require.Len(t, f.store.versions, 2)
	require.Equal(t, "more retries", f.store.versions[1].Message)
	require.Equal(t, []string{"queue.created", "queue.updated"}, f.events.actions)
}

func Test_Update_RefusesAChangeMadeOnAnOlderVersion(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	first, second := created.Settings, created.Settings
	first.MaxAttempts, second.RatePerSecond = 9, 2

	_, err := f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &first, BaseVersion: 1})
	_, stale := f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &second, BaseVersion: 1})

	require.NoError(t, err)
	require.ErrorIs(t, stale, apperror.Conflict(""))
	require.Len(t, f.store.versions, 2)
}

func Test_SetPaused(t *testing.T) {
	f := newFixture(t, false)
	f.usecase.Create(ctx, ana, draft())

	paused, err := f.usecase.SetPaused(ctx, ana, "prod", "place-import", true)

	require.NoError(t, err)
	require.True(t, paused.Paused)
	require.Contains(t, f.events.actions, "queue.paused")
}

func Test_Delete_RefusesAQueueWithMessagesUnlessForced(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = fresh(3)

	held := f.usecase.Delete(ctx, ana, "prod", "place-import", false)
	forced := f.usecase.Delete(ctx, ana, "prod", "place-import", true)

	require.ErrorIs(t, held, apperror.Conflict(""))
	require.NoError(t, forced)
	require.Empty(t, f.store.queues)
	require.Equal(t, entities.CommandDelete, f.store.commands[0].Kind)
}

func Test_Delete_RefusesUnknownDepthWithoutForce(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = entities.Stats{Depth: 0, ReportedAt: time.Now().Add(-2 * time.Minute)}

	err := f.usecase.Delete(ctx, ana, "prod", "place-import", false)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_Delete_AnEmptyQueue(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = fresh(0)

	require.NoError(t, f.usecase.Delete(ctx, ana, "prod", "place-import", false))
}

func Test_DeadLetterCommands(t *testing.T) {
	f := newFixture(t, false)
	f.usecase.Create(ctx, ana, draft())

	require.NoError(t, f.usecase.Replay(ctx, ana, "prod", "place-import", "m1"))
	require.NoError(t, f.usecase.ReplayAll(ctx, ana, "prod", "place-import"))
	require.NoError(t, f.usecase.PurgeDead(ctx, ana, "prod", "place-import"))

	kinds := []string{}
	for _, c := range f.store.commands {
		kinds = append(kinds, c.Kind)
	}
	require.Equal(t, []string{entities.CommandReplay, entities.CommandReplayAll, entities.CommandPurgeDead}, kinds)
	require.Equal(t, "m1", f.store.commands[0].Argument)
}

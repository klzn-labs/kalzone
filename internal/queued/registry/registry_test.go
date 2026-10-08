package registry_test

import (
	"testing"

	"github.com/adrianozp/platform/internal/queued/registry"
	"github.com/adrianozp/platform/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

func Test_Apply_StartsUpdatesAndStopsDispatchers(t *testing.T) {
	r, b, f := newRegistry()

	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1), apiQueue("q2", "b", 1)}})
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 2)}})

	require.True(t, f.made["q1"].started)
	require.False(t, f.made["q1"].stopped)
	require.Len(t, f.made["q1"].updates, 1)
	require.Equal(t, 2, f.made["q1"].updates[0].Version)
	require.True(t, f.made["q2"].stopped)
	require.Equal(t, []string{"ensure a", "ensure b", "ensure a"}, b.calls)
	require.Len(t, r.Queues(), 1)
}

func Test_Apply_ReconcilesOnPausedWithoutVersionChange(t *testing.T) {
	r, _, f := newRegistry()
	q := apiQueue("q1", "a", 1)
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{q}})
	q.Paused = true

	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{q}})

	require.Len(t, f.made["q1"].updates, 1)
	require.True(t, f.made["q1"].updates[0].Paused)
}

func Test_Apply_MissingQueueKeepsItsStreams(t *testing.T) {
	r, b, _ := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1)}})

	r.Apply(ctx, queuedapi.Desired{})

	require.NotContains(t, b.calls, "remove dev/a")
}

func Test_Apply_RunsCommandsAndReportsThemDone(t *testing.T) {
	r, b, _ := newRegistry()
	q := apiQueue("q1", "a", 1)

	done, errs := r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{q}, Commands: []queuedapi.Command{
		{ID: "c1", QueueID: "q1", Kind: "replay", Argument: "m1"},
		{ID: "c2", QueueID: "q1", Kind: "replay_all"},
		{ID: "c3", QueueID: "q1", Kind: "purge_dead"},
		{ID: "c4", QueueID: "q9", Kind: "delete", Argument: "dev/gone"},
	}})

	require.Empty(t, errs)
	require.ElementsMatch(t, []string{"c1", "c2", "c3", "c4"}, done)
	require.Contains(t, b.calls, "replay a m1")
	require.Contains(t, b.calls, "replay_all a")
	require.Contains(t, b.calls, "purge a")
	require.Contains(t, b.calls, "remove dev/gone")
}

func Test_Apply_SkipsAQueueWithBadSettings(t *testing.T) {
	r, _, _ := newRegistry()
	bad := apiQueue("q2", "b", 1)
	bad.Settings.Retention = "forever"

	_, errs := r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1), bad}})

	require.Len(t, errs, 1)
	require.Len(t, r.Queues(), 1)
}

func Test_Lookup_ByNameAndTokenHash(t *testing.T) {
	r, _, _ := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1)}})

	q, found := r.Lookup("a", "hash-a")
	_, unknownToken := r.Lookup("a", "hash-other")
	_, missing := r.Lookup("nope", "hash-a")

	require.Equal(t, registry.Found, found)
	require.Equal(t, "q1", q.ID)
	require.Equal(t, registry.Unauthorized, unknownToken)
	require.Equal(t, registry.NotFound, missing)
}

func Test_Apply_AReplayOfAGoneDeadLetterIsDone(t *testing.T) {
	r, _, _ := newRegistry()

	done, errs := r.Apply(ctx, queuedapi.Desired{
		Queues:   []queuedapi.Queue{apiQueue("q1", "a", 1)},
		Commands: []queuedapi.Command{{ID: "c1", QueueID: "q1", Kind: "replay", Argument: "gone"}},
	})

	require.Empty(t, errs)
	require.Equal(t, []string{"c1"}, done)
}

func Test_Apply_AFailedEnsureKeepsTheRunningQueue(t *testing.T) {
	r, b, f := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1)}})
	b.ensureFail = true

	_, errs := r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 2)}})

	require.Len(t, errs, 1)
	require.False(t, f.made["q1"].stopped)
	require.Empty(t, f.made["q1"].updates)
	require.Len(t, r.Queues(), 1)
	require.Equal(t, 1, r.Queues()[0].Version)
}

func Test_Apply_ACommandForADesiredQueueThatIsNotRunningWaits(t *testing.T) {
	r, b, _ := newRegistry()
	b.ensureFail = true

	done, errs := r.Apply(ctx, queuedapi.Desired{
		Queues:   []queuedapi.Queue{apiQueue("q1", "a", 1)},
		Commands: []queuedapi.Command{{ID: "c1", QueueID: "q1", Kind: "purge_dead"}, {ID: "c2", QueueID: "q9", Kind: "purge_dead"}},
	})

	require.Equal(t, []string{"c2"}, done)
	require.Len(t, errs, 2)
}

func Test_Apply_DeleteRunsBeforeAQueueRecreatedWithTheSameName(t *testing.T) {
	r, b, f := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1)}})

	r.Apply(ctx, queuedapi.Desired{
		Queues:   []queuedapi.Queue{apiQueue("q2", "a", 1)},
		Commands: []queuedapi.Command{{ID: "c1", QueueID: "q1", Kind: "delete", Argument: "dev/a"}},
	})

	require.Equal(t, []string{"ensure a", "remove dev/a", "ensure a"}, b.calls)
	require.True(t, f.made["q1"].stopped)
	require.True(t, f.made["q2"].started)
}

func Test_Apply_DeleteKeepsStreamsARunningQueueWithTheSameNameUses(t *testing.T) {
	r, b, _ := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q2", "a", 1)}})

	r.Apply(ctx, queuedapi.Desired{
		Queues:   []queuedapi.Queue{apiQueue("q2", "a", 1)},
		Commands: []queuedapi.Command{{ID: "c1", QueueID: "q1", Kind: "delete", Argument: "dev/a"}},
	})

	require.NotContains(t, b.calls, "remove dev/a")
}

func Test_Lookup_ATokenNoQueueKnowsIsUnauthorized(t *testing.T) {
	r, _, _ := newRegistry()
	r.Apply(ctx, queuedapi.Desired{Queues: []queuedapi.Queue{apiQueue("q1", "a", 1), apiQueue("q2", "b", 1)}})

	_, publishesElsewhere := r.Lookup("a", "hash-b")
	_, unknown := r.Lookup("a", "hash-nobody")
	_, unknownOnMissingQueue := r.Lookup("nope", "hash-nobody")

	require.Equal(t, registry.Forbidden, publishesElsewhere)
	require.Equal(t, registry.Unauthorized, unknown)
	require.Equal(t, registry.Unauthorized, unknownOnMissingQueue)
}

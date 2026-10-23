package cron_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/internal/queued/cron"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

func runs(ids ...string) []queuedapi.CronRun {
	result := []queuedapi.CronRun{}
	for _, id := range ids {
		result = append(result, queuedapi.CronRun{ID: id})
	}
	return result
}

func Test_Outbox_KeepsTheNewestRunsUpToItsLimit(t *testing.T) {
	o := cron.NewOutbox(2)
	for _, run := range runs("a", "b", "c") {
		o.Add(run)
	}

	require.Equal(t, runs("b", "c"), o.Drain())
	require.Empty(t, o.Drain())
}

func Test_Outbox_RestoresUnsentRunsBeforeNewerOnes(t *testing.T) {
	o := cron.NewOutbox(3)
	o.Add(queuedapi.CronRun{ID: "c"})

	o.Restore(runs("a", "b"))
	o.Restore(runs("x"))

	require.Equal(t, runs("a", "b", "c"), o.Drain())
}

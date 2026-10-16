package queues_test

import (
	"testing"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/stretchr/testify/require"
)

func Test_Detach_DropsConsumedQueuesAndThePublisherAndRevokesItsToken(t *testing.T) {
	f := newFixture(t, false)
	consumed := entities.Queue{ID: "q1", Name: "emails", Environment: "dev", ConsumerID: "s-gone", Version: 1, Publishers: []string{"s-gone"}}
	published := entities.Queue{ID: "q2", Name: "place-import", Environment: "prod", ConsumerID: "s-worker", Version: 4, Publishers: []string{"s-worker", "s-gone"}}
	untouched := entities.Queue{ID: "q3", Name: "reports", Environment: "prod", ConsumerID: "s-worker", Version: 2, Publishers: []string{"s-worker"}}
	for _, q := range []entities.Queue{consumed, published, untouched} {
		f.store.queues[q.Environment+"/"+q.Name] = q
	}

	require.NoError(t, f.usecase.Detach(ctx, ana, "s-gone"))

	_, stillThere := f.store.queues["dev/emails"]
	require.False(t, stillThere)
	require.Equal(t, entities.CommandDelete, f.store.commands[0].Kind)
	require.Equal(t, []string{"s-worker"}, f.store.queues["prod/place-import"].Publishers)
	require.Equal(t, 5, f.store.queues["prod/place-import"].Version)
	require.Equal(t, 2, f.store.queues["prod/reports"].Version)
	require.Equal(t, []string{"s-gone"}, f.store.revoked)
	require.ElementsMatch(t, []string{"queue.deleted", "queue.updated"}, f.events.actions)
}

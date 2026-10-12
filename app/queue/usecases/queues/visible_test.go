package queues_test

import (
	"testing"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ReadsFor_SkipQueuesOfOtherTeams(t *testing.T) {
	f := newFixture(t, false)
	f.store.queues[key("dev", "mine")] = entities.Queue{ID: "q1", Name: "mine", Environment: "dev", TeamID: "t1"}
	f.store.queues[key("dev", "theirs")] = entities.Queue{ID: "q2", Name: "theirs", Environment: "dev", TeamID: "t2"}
	agent := ana
	agent.Token = actor.Token{ID: "tok", TeamIDs: []string{"t1"}}

	list, err := f.usecase.ListFor(ctx, agent, "dev", "")
	require.NoError(t, err)
	_, hidden := f.usecase.GetFor(ctx, agent, "dev", "theirs")
	_, _, hiddenStats := f.usecase.StatsFor(ctx, agent, "dev", "theirs")
	all, _ := f.usecase.ListFor(ctx, ana, "dev", "")

	require.Len(t, list, 1)
	require.Equal(t, "mine", list[0].Name)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.ErrorIs(t, hiddenStats, apperror.NotFound(""))
	require.Len(t, all, 2)
}

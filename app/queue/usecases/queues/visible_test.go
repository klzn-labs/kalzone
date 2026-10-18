package queues_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/page"
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

func Test_VersionPageFor_PagesNewestFirst(t *testing.T) {
	f := newFixture(t, false)
	f.store.queues[key("dev", "mine")] = entities.Queue{ID: "q1", Name: "mine", Environment: "dev", TeamID: "t1"}
	f.store.versions = []entities.Version{{Number: 3}, {Number: 2}, {Number: 1}}

	first, err := f.usecase.VersionPageFor(ctx, ana, "dev", "mine", page.First(2))
	require.NoError(t, err)
	next, _ := page.Parse("2", first.Next)
	second, err := f.usecase.VersionPageFor(ctx, ana, "dev", "mine", next)
	require.NoError(t, err)

	require.Equal(t, []int{3, 2}, []int{first.Items[0].Number, first.Items[1].Number})
	require.Equal(t, []entities.Version{{Number: 1}}, second.Items)
	require.Empty(t, second.Next)
}

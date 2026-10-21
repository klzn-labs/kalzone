package overview

import (
	"testing"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/stretchr/testify/require"
)

func lane(name string, from, to int) Lane {
	return Lane{Service: catalog.Service{Name: name}, From: Stage{Release: from}, To: Stage{Release: to}}
}

func Test_LaneCountsTheReleasesProdIsBehind(t *testing.T) {
	require.Equal(t, 2, lane("a", 26, 24).Behind())
	require.Equal(t, 0, lane("b", 9, 9).Behind())
	require.Equal(t, 0, lane("c", 3, 5).Behind())
	require.Equal(t, 0, lane("d", 4, 0).Behind())
	require.True(t, lane("d", 4, 0).NotPromoted())
	require.False(t, lane("e", 0, 0).Waits())
}

func Test_LanesThatWaitComeFirst(t *testing.T) {
	lanes := []Lane{lane("api", 9, 9), lane("web", 2, 0), lane("grid", 26, 25), lane("site", 1, 1)}

	sortLanes(lanes)

	require.Equal(t, []string{"grid", "web", "api", "site"}, []string{lanes[0].Service.Name, lanes[1].Service.Name, lanes[2].Service.Name, lanes[3].Service.Name})
	require.Equal(t, 2, waiting(lanes))
	require.Equal(t, 2, inStep(append(lanes, lane("new", 0, 0))))
}

func Test_PromotionGoesFromTheEnvironmentBeforeTheFirstProtectedOne(t *testing.T) {
	envs := []catalog.Environment{{Slug: "prod", Protected: true, Position: 2}, {Slug: "dev", Position: 0}, {Slug: "staging", Position: 1}}

	from, to, ok := promotionPair(envs)
	_, _, none := promotionPair([]catalog.Environment{{Slug: "dev"}})

	require.True(t, ok)
	require.Equal(t, []string{"staging", "prod"}, []string{from.Slug, to.Slug})
	require.False(t, none)
}

package source_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/team/usecases/source"
	"github.com/stretchr/testify/require"
)

type memberships map[string][]entities.Membership

func (m memberships) Membership(context.Context, string, string) (entities.Membership, bool, error) {
	return entities.Membership{}, false, nil
}

func (m memberships) MembershipsOfUser(_ context.Context, userID string) ([]entities.Membership, error) {
	return m[userID], nil
}

func (m memberships) MembershipsOfTeam(context.Context, string) ([]entities.Membership, error) {
	return nil, nil
}

func (m memberships) DemoTeamIDs(context.Context) ([]string, error) {
	return []string{"demo"}, nil
}

func Test_VisibleTeamIDs_ShowsTheDemoTeamOnlyToPeopleOnNoTeam(t *testing.T) {
	s := source.NewLocalTeamSource(memberships{"u1": {{TeamID: "t1", UserID: "u1"}}})

	member, err := s.VisibleTeamIDs(context.Background(), "u1")
	require.NoError(t, err)
	newcomer, _ := s.VisibleTeamIDs(context.Background(), "u2")

	require.Equal(t, []string{"t1"}, member)
	require.Equal(t, []string{"demo"}, newcomer)
}

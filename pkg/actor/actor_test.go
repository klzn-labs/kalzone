package actor_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/stretchr/testify/require"
)

func Test_Sees_MemberOnlyTheirTeams(t *testing.T) {
	ana := actor.Actor{UserID: "u1", Teams: []string{"t1"}}

	require.True(t, ana.Sees("t1"))
	require.False(t, ana.Sees("t2"))
}

func Test_Sees_NewcomerNothing(t *testing.T) {
	require.False(t, actor.Actor{UserID: "u1"}.Sees("t1"))
}

func Test_Sees_AdminAndSystemEverything(t *testing.T) {
	require.True(t, actor.Actor{UserID: "u1", Admin: true}.Sees("t2"))
	require.True(t, actor.Actor{Login: "auto-deploy"}.Sees("t2"))
}

func Test_Sees_TokenIntersectsOwnerTeams(t *testing.T) {
	ana := actor.Actor{UserID: "u1", Teams: []string{"t1", "t2"}, Token: actor.Token{ID: "k1", TeamIDs: []string{"t2", "t3"}}}

	teams, all := ana.TeamScope()

	require.False(t, ana.Sees("t1"))
	require.True(t, ana.Sees("t2"))
	require.False(t, ana.Sees("t3"))
	require.Equal(t, []string{"t2"}, teams)
	require.False(t, all)
}

func Test_TeamScope_AdminSessionSeesAllTeams(t *testing.T) {
	_, all := actor.Actor{UserID: "u1", Admin: true}.TeamScope()

	require.True(t, all)
}

func Test_TeamScope_AdminTokenStaysWithinTokenTeams(t *testing.T) {
	teams, all := actor.Actor{UserID: "u1", Admin: true, Token: actor.Token{ID: "k1", TeamIDs: []string{"t3"}}}.TeamScope()

	require.Equal(t, []string{"t3"}, teams)
	require.False(t, all)
}

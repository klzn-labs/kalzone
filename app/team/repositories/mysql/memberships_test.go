package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/team/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func Test_PutMembership_UpsertsTheRole(t *testing.T) {
	repo := mysql.NewTeamRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, entities.Team{ID: "t1", Slug: "places", Name: "Places", CreatedAt: time.Now()}))
	require.NoError(t, repo.PutMembership(ctx, entities.Membership{TeamID: "t1", UserID: "u1", Role: entities.RoleMember, CreatedAt: time.Now()}))
	require.NoError(t, repo.PutMembership(ctx, entities.Membership{TeamID: "t1", UserID: "u1", Role: entities.RoleMaintainer, CreatedAt: time.Now()}))

	m, found, err := repo.Membership(ctx, "t1", "u1")
	count, countErr := repo.CountMaintainers(ctx, "t1")

	require.NoError(t, err)
	require.NoError(t, countErr)
	require.True(t, found)
	require.Equal(t, entities.RoleMaintainer, m.Role)
	require.EqualValues(t, 1, count)
}

func Test_Create_DuplicateSlugIsAConflict(t *testing.T) {
	repo := mysql.NewTeamRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, entities.Team{ID: "t1", Slug: "places", Name: "Places", CreatedAt: time.Now()}))

	err := repo.Create(ctx, entities.Team{ID: "t2", Slug: "places", Name: "Other", CreatedAt: time.Now()})

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_Delete_RemovesMemberships(t *testing.T) {
	repo := mysql.NewTeamRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, entities.Team{ID: "t1", Slug: "places", Name: "Places", CreatedAt: time.Now()}))
	require.NoError(t, repo.PutMembership(ctx, entities.Membership{TeamID: "t1", UserID: "u1", Role: entities.RoleMember, CreatedAt: time.Now()}))

	require.NoError(t, repo.Delete(ctx, "t1"))

	memberships, err := repo.MembershipsOfUser(ctx, "u1")
	require.NoError(t, err)
	require.Empty(t, memberships)
}

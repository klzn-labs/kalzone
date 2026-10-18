package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/catalog/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func service(id, slug, team string) entities.Service {
	return entities.Service{ID: id, Slug: slug, Name: slug, OwnerTeamID: team, CreatedAt: time.Now().UTC()}
}

func slugs(services []entities.Service) []string {
	result := []string{}
	for _, s := range services {
		result = append(result, s.Slug)
	}
	return result
}

func Test_SoftDeletedServicesAreHiddenAndFreeTheirSlug(t *testing.T) {
	repo := mysql.NewServiceRepository(testdb.Open(t))
	gone := service("s1", "api", "t1")
	require.NoError(t, repo.Create(ctx, gone))
	now := time.Now().UTC()
	gone.DeletedAt = &now
	require.NoError(t, repo.Update(ctx, gone))

	_, findErr := repo.FindBySlug(ctx, "api")
	reuseErr := repo.Create(ctx, service("s2", "api", "t1"))
	reused, reusedErr := repo.FindBySlug(ctx, "api")
	takenErr := repo.Create(ctx, service("s3", "api", "t1"))

	require.ErrorIs(t, findErr, apperror.NotFound(""))
	require.NoError(t, reuseErr)
	require.NoError(t, reusedErr)
	require.Equal(t, "s2", reused.ID)
	require.ErrorIs(t, takenErr, apperror.Conflict(""))
}

func Test_ADecommissioningServiceStaysVisibleWithItsStatus(t *testing.T) {
	repo := mysql.NewServiceRepository(testdb.Open(t))
	api := service("s1", "api", "t1")
	require.NoError(t, repo.Create(ctx, api))
	created, _ := repo.FindByID(ctx, "s1")
	api.Status = entities.StatusDecommissioning
	require.NoError(t, repo.Update(ctx, api))

	found, err := repo.FindBySlug(ctx, "api")

	require.NoError(t, err)
	require.Equal(t, entities.StatusActive, created.Status)
	require.True(t, found.Decommissioning())
}

func Test_List_FiltersByOwnerTeams(t *testing.T) {
	repo := mysql.NewServiceRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, service("s1", "api", "t1")))
	require.NoError(t, repo.Create(ctx, service("s2", "web", "t2")))

	all, err := repo.List(ctx, entities.ServiceFilter{})
	require.NoError(t, err)
	mine, _ := repo.List(ctx, entities.ServiceFilter{OwnerTeamIDs: []string{"t2"}})
	none, _ := repo.List(ctx, entities.ServiceFilter{OwnerTeamIDs: []string{}})
	count, _ := repo.CountOwnedBy(ctx, "t1")

	require.Equal(t, []string{"api", "web"}, slugs(all))
	require.Equal(t, []string{"web"}, slugs(mine))
	require.Empty(t, none)
	require.EqualValues(t, 1, count)
}

func Test_Environments_AreSeededInOrder(t *testing.T) {
	repo := mysql.NewEnvironmentRepository(testdb.Open(t))

	envs, err := repo.List(ctx)

	require.NoError(t, err)
	require.Len(t, envs, 2)
	require.Equal(t, entities.Environment{Slug: "dev", Name: "Development", Protected: false, Position: 1}, envs[0])
	require.True(t, envs[1].Protected)
}

func Test_List_FiltersByGitHubRepo(t *testing.T) {
	repo := mysql.NewServiceRepository(testdb.Open(t))
	api := service("s1", "api", "t1")
	api.GitHubRepo, api.AutoBuild, api.AutoDeployDev, api.BuildWorkflow = "o/r", true, true, "platform-build.yml"
	require.NoError(t, repo.Create(ctx, api))
	require.NoError(t, repo.Create(ctx, service("s2", "web", "t1")))

	found, err := repo.List(ctx, entities.ServiceFilter{GitHubRepo: "o/r"})

	require.NoError(t, err)
	require.Equal(t, []string{"api"}, slugs(found))
	require.True(t, found[0].AutoBuild)
	require.True(t, found[0].AutoDeployDev)
}

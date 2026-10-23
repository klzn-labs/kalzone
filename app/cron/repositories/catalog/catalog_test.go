package catalog_test

import (
	"context"
	"testing"

	catalogEntities "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/cron/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var worker = catalogEntities.Service{ID: "s1", Slug: "worker", OwnerTeamID: "t1", Status: catalogEntities.StatusDecommissioning}

type services struct{}

func (services) Get(context.Context, string) (catalogEntities.Service, error) { return worker, nil }

func (services) GetByID(context.Context, string) (catalogEntities.Service, error) { return worker, nil }

type environments struct{}

func (environments) Get(_ context.Context, slug string) (catalogEntities.Environment, error) {
	if slug != "dev" {
		return catalogEntities.Environment{}, apperror.NotFound("environment not found")
	}
	return catalogEntities.Environment{Slug: "dev"}, nil
}

type teams struct{}

func (teams) Get(context.Context, string) (team.Team, error) { return team.Team{ID: "t1"}, nil }

func lookup() catalog.Lookup {
	return catalog.NewLookup(services{}, environments{}, teams{})
}

func Test_Lookup_MapsServicesTeamsAndEnvironments(t *testing.T) {
	var _ crons.Catalog = lookup()
	var _ executor.ServiceLookup = lookup()

	svc, err := lookup().FindService(ctx, "worker")
	byID, _ := lookup().FindServiceByID(ctx, "s1")
	teamID, _ := lookup().FindTeam(ctx, "plateful")
	slug, _ := lookup().SlugOf(ctx, "s1")

	require.NoError(t, err)
	require.Equal(t, crons.ServiceRef{ID: "s1", Slug: "worker", OwnerTeamID: "t1", Decommissioning: true}, svc)
	require.Equal(t, svc, byID)
	require.Equal(t, "t1", teamID)
	require.Equal(t, "worker", slug)
	require.NoError(t, lookup().EnvironmentExists(ctx, "dev"))
	require.ErrorIs(t, lookup().EnvironmentExists(ctx, "qa"), apperror.NotFound(""))
}

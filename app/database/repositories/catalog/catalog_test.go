package catalog_test

import (
	"context"
	"testing"

	catalogEntities "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/database/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var shop = catalogEntities.Service{ID: "s1", Slug: "shop", OwnerTeamID: "t1"}

type services struct{}

func (services) Get(context.Context, string) (catalogEntities.Service, error) { return shop, nil }

func (services) GetByID(context.Context, string) (catalogEntities.Service, error) { return shop, nil }

type teams struct{}

func (teams) Get(context.Context, string) (team.Team, error) { return team.Team{ID: "t1"}, nil }

func (teams) FindByIDs(_ context.Context, ids []string) ([]team.Team, error) {
	if ids[0] != "t1" {
		return []team.Team{}, nil
	}
	return []team.Team{{ID: "t1", Slug: "plateful"}}, nil
}

type protection struct{}

func (protection) IsProtected(_ context.Context, env string) (bool, error) { return env == "prod", nil }

func lookup() catalog.Lookup {
	return catalog.NewLookup(services{}, teams{}, protection{})
}

func Test_Lookup_MapsServicesAndTeams(t *testing.T) {
	var _ databases.Catalog = lookup()
	svc, err := lookup().FindService(ctx, "shop")
	require.NoError(t, err)
	require.Equal(t, databases.ServiceRef{ID: "s1", Slug: "shop", OwnerTeamID: "t1"}, svc)
	byID, _ := lookup().FindServiceByID(ctx, "s1")
	require.Equal(t, svc, byID)
	teamID, _ := lookup().FindTeam(ctx, "plateful")
	require.Equal(t, "t1", teamID)
	protected, _ := lookup().IsProtected(ctx, "prod")
	require.True(t, protected)
}

func Test_TeamSlug(t *testing.T) {
	slug, err := lookup().TeamSlug(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "plateful", slug)

	_, err = lookup().TeamSlug(ctx, "gone")
	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
}

package catalog_test

import (
	"context"
	"testing"

	catalogEntities "github.com/klzn-labs/kalzone/app/catalog/entities"
	database "github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/repositories/catalog"
	"github.com/stretchr/testify/require"
)

type lister []catalogEntities.Service

func (l lister) List(context.Context, catalogEntities.ServiceFilter) ([]catalogEntities.Service, error) {
	return l, nil
}

type attachments map[string][]database.Database

func (a attachments) AttachedTo(_ context.Context, serviceID string) ([]database.Database, error) {
	return a[serviceID], nil
}

func Test_Services_MapsAttachmentsPerService(t *testing.T) {
	services := lister{{ID: "s1", Slug: "api", Name: "API"}, {ID: "s2", Slug: "web", Name: "Web"}}
	attached := attachments{"s1": {{Environment: "prod", Schema: "plateful", Username: "kz_plateful", MaxConnections: 30}}}

	got, err := catalog.NewServices(services, attached).Services(context.Background())

	require.NoError(t, err)
	require.Equal(t, []entities.Service{
		{Slug: "api", Name: "API", Databases: []entities.Database{{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30}}},
		{Slug: "web", Name: "Web", Databases: []entities.Database{}},
	}, got)
}

func Test_Services_LeavesOutServicesBeingDeletedOrSimulated(t *testing.T) {
	services := lister{{ID: "s1", Slug: "api"}, {ID: "s2", Slug: "old", Status: catalogEntities.StatusDecommissioning}, {ID: "s3", Slug: "demo-shop", Simulated: true}}

	got, err := catalog.NewServices(services, attachments{}).Services(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "api", got[0].Slug)
}

func Test_Simulated_ListsOnlyTheDemosLiveServices(t *testing.T) {
	services := lister{{ID: "s1", Slug: "api"}, {ID: "s2", Slug: "demo-shop", Name: "Shop", OwnerTeamID: "d1", Simulated: true}}

	got, err := catalog.NewServices(services, attachments{}).Simulated(context.Background())

	require.NoError(t, err)
	require.Equal(t, []entities.Service{{Slug: "demo-shop", Name: "Shop", TeamID: "d1"}}, got)
}

package dashboards_test

import (
	"context"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/dashboards"
	"github.com/stretchr/testify/require"
)

type catalog struct {
	services []entities.Service
	calls    int
}

func (c *catalog) Services(context.Context) ([]entities.Service, error) {
	c.calls++
	return c.services, nil
}

type publisher struct {
	enabled   bool
	err       error
	published []entities.Service
}

func (p *publisher) Enabled() bool { return p.enabled }
func (p *publisher) PublishAttachments(_ context.Context, services []entities.Service) error {
	p.published = services
	return p.err
}

func apiAndWeb() *catalog {
	return &catalog{services: []entities.Service{
		{Slug: "api", Databases: []entities.Database{{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30}}},
		{Slug: "web"},
	}}
}

func Test_Sync_WritesEveryDashboardAndPublishesTheAttachments(t *testing.T) {
	g, p := &grafana{enabled: true}, &publisher{enabled: true}

	require.NoError(t, dashboards.NewDashboardsUsecase(g, apiAndWeb(), p, &orgs{}).Sync(context.Background()))

	require.Equal(t, []string{"platform-api", "platform-web", "kalzone-controlplane"}, g.written)
	require.Equal(t, []string{"api", "web"}, []string{p.published[0].Slug, p.published[1].Slug})
}

func Test_Sync_PublishesEvenWhenGrafanaIsOff(t *testing.T) {
	g, p := &grafana{}, &publisher{enabled: true}

	require.NoError(t, dashboards.NewDashboardsUsecase(g, apiAndWeb(), p, &orgs{}).Sync(context.Background()))

	require.Empty(t, g.written)
	require.Len(t, p.published, 2)
}

func Test_Sync_WritesDashboardsWhenThePublisherFails(t *testing.T) {
	g, p := &grafana{enabled: true}, &publisher{enabled: true, err: errors.New("pushgateway down")}

	err := dashboards.NewDashboardsUsecase(g, apiAndWeb(), p, &orgs{}).Sync(context.Background())

	require.ErrorContains(t, err, "pushgateway down")
	require.Len(t, g.written, 3)
}

func Test_Sync_WritesThePlatformDashboardWithoutServices(t *testing.T) {
	g := &grafana{enabled: true}

	require.NoError(t, dashboards.NewDashboardsUsecase(g, &catalog{}, &publisher{}, &orgs{}).Sync(context.Background()))

	require.Equal(t, []string{"kalzone-controlplane"}, g.written)
}

func Test_Sync_SkipsADisabledPublisher(t *testing.T) {
	p := &publisher{}

	require.NoError(t, dashboards.NewDashboardsUsecase(&grafana{enabled: true}, apiAndWeb(), p, &orgs{}).Sync(context.Background()))

	require.Nil(t, p.published)
}

func Test_SyncsOn_ServiceAndDatabaseEvents(t *testing.T) {
	u := dashboards.NewDashboardsUsecase(&grafana{}, &catalog{}, &publisher{}, &orgs{})

	for _, action := range []string{"service.created", "service.updated", "database.attached", "database.detached", "database.adopted", "database.deleted", "database.released"} {
		require.True(t, u.SyncsOn(action), action)
	}
	require.False(t, u.SyncsOn("queue.created"))
}

func Test_WithSharing_NamesServicesOnTheSameSchema(t *testing.T) {
	prod := entities.Database{Environment: "prod", Schema: "plateful"}
	services := dashboards.WithSharing([]entities.Service{
		{Slug: "api", Databases: []entities.Database{prod, {Environment: "dev", Schema: "plateful_dev"}}},
		{Slug: "worker", Databases: []entities.Database{prod}},
		{Slug: "importer", Databases: []entities.Database{prod}},
	})

	require.Equal(t, []string{"importer", "worker"}, services[0].Databases[0].SharedWith)
	require.Empty(t, services[0].Databases[1].SharedWith)
	require.Equal(t, []string{"api", "importer"}, services[1].Databases[0].SharedWith)
}

func Test_Sync_HandsTheSharedServicesToTheTeamOrgs(t *testing.T) {
	o := &orgs{}

	require.NoError(t, dashboards.NewDashboardsUsecase(&grafana{enabled: true}, apiAndWeb(), &publisher{}, o).Sync(context.Background()))

	require.Equal(t, []string{"api", "web"}, []string{o.synced[0].Slug, o.synced[1].Slug})
}

func Test_SyncsOn_TeamAndAdminChanges(t *testing.T) {
	u := dashboards.NewDashboardsUsecase(&grafana{}, &catalog{}, &publisher{}, &orgs{})

	for _, action := range []string{"team.created", "team.deleted", "team.member_put", "team.member_removed", "user.admin_granted", "user.admin_revoked"} {
		require.True(t, u.SyncsOn(action), action)
	}
}

package entities_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

var teamA = entities.Scope{
	Services: []string{"api", "web.v2"}, Environments: []string{"dev", "prod"},
	MySQLUsers: []string{"api_prod"}, Schemas: []string{"api"},
}

func scoped(t *testing.T, query string, s entities.Scope) string {
	t.Helper()
	out, err := entities.ScopeQuery(query, s)
	require.NoError(t, err)
	return out
}

func Test_ScopeQuery_LimitsTraefikAndContainerSeriesToTheTeamsDeployments(t *testing.T) {
	traefik := scoped(t, `sum(rate(traefik_service_requests_total{code=~"5.."}[5m]))`, teamA)
	memory := scoped(t, `container_memory_working_set_bytes`, teamA)

	require.Equal(t, `sum(rate(traefik_service_requests_total{code=~"5..",service=~"(?:api|web\\.v2)-(?:dev|prod)-(?:blue|green)@file"}[5m]))`, traefik)
	require.Equal(t, `container_memory_working_set_bytes{name=~"(?:api|web\\.v2)-(?:dev|prod)-(?:blue|green)"}`, memory)
}

func Test_ScopeQuery_LimitsEverySelectorOnBothSidesOfAJoin(t *testing.T) {
	query := scoped(t, `sum(mysql_info_schema_processlist_processes_by_user and on(mysql_user) kalzone_database_attachment{service="api"})`, teamA)

	require.Contains(t, query, `mysql_info_schema_processlist_processes_by_user{mysql_user=~"(?:api_prod)"}`)
	require.Contains(t, query, `kalzone_database_attachment{service="api",service=~"(?:api|web\\.v2)"}`)
}

func Test_ScopeQuery_LimitsStatementsBySchemaAndLeavesServerWideCountersAlone(t *testing.T) {
	require.Equal(t, `mysql_perf_schema_events_statements_total{schema=~"(?:api)"}`, scoped(t, `mysql_perf_schema_events_statements_total`, teamA))
	require.Equal(t, `mysql_global_status_threads_connected`, scoped(t, `mysql_global_status_threads_connected`, teamA))
}

func Test_ScopeQuery_LetsTheSharedDatabaseContainerThrough(t *testing.T) {
	query := `rate(container_cpu_usage_seconds_total{name="plateful-mysql-1"}[5m])`

	require.Equal(t, query, scoped(t, query, teamA))
}

func Test_ScopeQuery_EmptiesSelectorsWithoutAKnownMetricName(t *testing.T) {
	require.Equal(t, `{__name__="kalzone:out_of_scope",service=~".+"}`, scoped(t, `{service=~".+"}`, teamA))
	require.Equal(t, `{__name__="kalzone:out_of_scope",__name__=~"traefik.*"}`, scoped(t, `{__name__=~"traefik.*"}`, teamA))
	require.Equal(t, `up{__name__="kalzone:out_of_scope"}`, scoped(t, `up`, teamA))
}

func Test_ScopeQuery_EmptiesEverythingForATeamWithNoServices(t *testing.T) {
	query := scoped(t, `traefik_service_requests_total`, entities.Scope{Environments: []string{"dev"}})

	require.Equal(t, `traefik_service_requests_total{__name__="kalzone:out_of_scope"}`, query)
}

func Test_ScopeQuery_RejectsWhatIsNotPromQL(t *testing.T) {
	_, err := entities.ScopeQuery(`sum(`, teamA)

	require.Error(t, err)
}

func Test_ScopeSelector_LimitsAMatchParameter(t *testing.T) {
	out, err := entities.ScopeSelector(`kalzone_database_attachment`, teamA)

	require.NoError(t, err)
	require.Equal(t, `{__name__="kalzone_database_attachment",service=~"(?:api|web\\.v2)"}`, out)
}

func Test_ScopeDefaultSelectors_CoverEveryFamilyTheTeamMayRead(t *testing.T) {
	selectors := entities.ScopeDefaultSelectors(teamA)

	require.Contains(t, selectors, `{__name__=~"mysql_global_.*"}`)
	require.Contains(t, selectors, `{__name__=~"kalzone_database_attachment.*",service=~"(?:api|web\\.v2)"}`)
}

func Test_Tenants_AreEveryServiceInEveryEnvironment(t *testing.T) {
	require.Equal(t, []string{"api-dev", "api-prod", "web.v2-dev", "web.v2-prod"}, teamA.Tenants())
}

func Test_TeamScope_TakesOnlyTheTeamsServicesAndTheirDatabasesOnce(t *testing.T) {
	services := []entities.Service{
		{Slug: "api", TeamID: "t1", Databases: []entities.Database{{Schema: "shop", User: "api_dev"}, {Schema: "shop", User: "api_prod"}}},
		{Slug: "worker", TeamID: "t1", Databases: []entities.Database{{Schema: "shop", User: "api_prod"}}},
		{Slug: "other", TeamID: "t2", Databases: []entities.Database{{Schema: "other", User: "other"}}},
	}

	s := entities.TeamScope(services, "t1", []string{"dev"})

	require.Equal(t, []string{"api", "worker"}, s.Services)
	require.Equal(t, []string{"api_dev", "api_prod"}, s.MySQLUsers)
	require.Equal(t, []string{"shop"}, s.Schemas)
}

func Test_AllTenants_AddsThePlatformAndTheLogsFromBeforeTenants(t *testing.T) {
	tenants := entities.AllTenants([]entities.Service{{Slug: "api"}}, []string{"dev"})

	require.Equal(t, []string{"api-dev", "controlplane", "platform", "fake"}, tenants)
}

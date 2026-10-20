package entities_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_Dashboard_QueriesTheServicesWorkloadsPerEnvironment(t *testing.T) {
	raw, err := json.Marshal(entities.Dashboard(entities.Service{Slug: "places-api", Name: "Places API"}))
	require.NoError(t, err)
	body := string(raw)

	require.Contains(t, body, `"uid":"platform-places-api"`)
	require.Contains(t, body, `service=~\"places-api-${env}-(blue|green)@file\"`)
	require.Contains(t, body, `name=~\"places-api-${env}-(blue|green)\"`)
	require.Contains(t, body, `{container=~\"places-api-${env}-(blue|green)\"}`)
	require.Contains(t, body, `plateful_traffic_weight_percent{service=\"places-api-${env}\"}`)
	require.Contains(t, body, `"lineInterpolation":"stepAfter"`)
	require.Contains(t, body, `"uid":"loki"`)
	require.Contains(t, body, `"query":"dev,prod"`)
}

func dashboardJSON(t *testing.T, s entities.Service) string {
	t.Helper()
	raw, err := json.Marshal(entities.Dashboard(s))
	require.NoError(t, err)
	return string(raw)
}

func Test_Dashboard_HasNoDatabaseRowWithoutAttachments(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "web", Name: "Web"})

	require.NotContains(t, body, "kalzone_database_attachment")
	require.NotContains(t, body, `"type":"row"`)
	require.Contains(t, body, `"gridPos":{"h":12,"w":24,"x":0,"y":16}`)
}

func Test_Dashboard_AddsTheDatabaseRowFilteredByServiceAndEnv(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "api", Name: "API", Databases: []entities.Database{
		{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30},
	}})

	require.Contains(t, body, `"type":"row"`)
	require.Contains(t, body, `"title":"Database"`)
	require.Contains(t, body, `kalzone_database_attachment{service=\"api\",environment=\"$env\"}`)
	require.Contains(t, body, `and on(mysql_user)`)
	require.Contains(t, body, `and on(schema)`)
	require.Contains(t, body, `container_cpu_usage_seconds_total{name=\"plateful-mysql-1\"}`)
	require.Contains(t, body, "Shared by every service on this MySQL server")
	require.Contains(t, body, `increase(mysql_perf_schema_events_statements_seconds_total[$__range])`)
	require.Contains(t, body, `"title":"Slowest queries"`)
	require.Contains(t, body, `"gridPos":{"h":12,"w":24,"x":0,"y":35}`)
}

func Test_Dashboard_NeverMatchesAnotherServicesAttachment(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "api", Databases: []entities.Database{{Environment: "prod"}}})

	require.Equal(t, strings.Count(body, "kalzone_database_attachment{"), strings.Count(body, `kalzone_database_attachment{service=\"api\",environment=\"$env\"}`))
}

func Test_DatabaseRowTitle_NamesTheOtherServices(t *testing.T) {
	require.Equal(t, "Database", entities.DatabaseRowTitle([]entities.Database{{}}))
	require.Equal(t, "Database (shared with importer, worker)", entities.DatabaseRowTitle([]entities.Database{
		{Environment: "prod", SharedWith: []string{"worker", "importer"}}, {Environment: "dev", SharedWith: []string{"worker"}},
	}))
}

package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/adrianozp/platform/app/observe/entities"
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

package grafana_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

func server(t *testing.T, body string, status int, seen *url.URL) grafana.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = *r.URL
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return grafana.NewClient(config.Config{Grafana: config.Grafana{URL: s.URL + "/grafana", Token: "token"}})
}

func Test_Instant_ReadsTheFirstSampleOfAVector(t *testing.T) {
	var seen url.URL
	c := server(t, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"12.5"]}]}}`, 200, &seen)

	value, found, err := c.Instant(context.Background(), "prom", `up{name="x"}`)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 12.5, value)
	require.Equal(t, "/grafana/api/datasources/proxy/uid/prom/api/v1/query", seen.Path)
	require.Equal(t, `up{name="x"}`, seen.Query().Get("query"))
}

func Test_Instant_FindsNothingInAnEmptyVector(t *testing.T) {
	var seen url.URL
	c := server(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`, 200, &seen)

	_, found, err := c.Instant(context.Background(), "prom", "up")

	require.NoError(t, err)
	require.False(t, found)
}

func Test_LogLines_ReturnsEveryStreamOldestFirst(t *testing.T) {
	var seen url.URL
	c := server(t, `{"status":"success","data":{"resultType":"streams","result":[
		{"stream":{"container":"api-dev-blue"},"values":[["1700000002000000000","b2"],["1700000000000000000","b0"]]},
		{"stream":{"container":"api-dev-green"},"values":[["1700000001000000000","g1"]]}]}}`, 200, &seen)
	end := time.Unix(1700000100, 0)

	lines, err := c.LogLines(context.Background(), "loki", `{container="x"}`, end.Add(-time.Hour), end, 10)

	require.NoError(t, err)
	require.Equal(t, []string{"b0", "g1", "b2"}, []string{lines[0].Line, lines[1].Line, lines[2].Line})
	require.Equal(t, "api-dev-green", lines[1].Container)
	require.Equal(t, time.Unix(1700000001, 0).UTC(), lines[1].At)
	require.Equal(t, "/grafana/api/datasources/proxy/uid/loki/loki/api/v1/query_range", seen.Path)
	require.Equal(t, "backward", seen.Query().Get("direction"))
	require.Equal(t, "10", seen.Query().Get("limit"))
	require.Equal(t, "1700000100000000000", seen.Query().Get("end"))
}

func Test_Queries_FailAsUpstreamOrNotConfigured(t *testing.T) {
	var seen url.URL
	broken := server(t, `oops`, 500, &seen)
	unset := grafana.NewClient(config.Config{})

	_, _, upstream := broken.Instant(context.Background(), "prom", "up")
	_, missing := unset.LogLines(context.Background(), "loki", "{}", time.Now(), time.Now(), 1)

	require.Equal(t, apperror.CodeUpstream, apperror.CodeOf(upstream))
	require.Equal(t, apperror.CodeNotConfigured, apperror.CodeOf(missing))
}

func Test_DeleteDashboard_RemovesByUIDAndToleratesAMissingOne(t *testing.T) {
	var seen url.URL
	deleted := server(t, `{"title":"api"}`, http.StatusOK, &seen)
	missing := server(t, `{"message":"Dashboard not found"}`, http.StatusNotFound, &seen)
	broken := server(t, `{}`, http.StatusForbidden, &seen)

	require.NoError(t, deleted.DeleteDashboard(context.Background(), "platform-api"))
	require.Equal(t, "/grafana/api/dashboards/uid/platform-api", seen.Path)
	require.NoError(t, missing.DeleteDashboard(context.Background(), "platform-api"))
	require.Error(t, broken.DeleteDashboard(context.Background(), "platform-api"))
}

func Test_Range_ReadsTheFirstSeriesOfAMatrix(t *testing.T) {
	var seen url.URL
	c := server(t, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"0.18"],[1700000300,"NaN"],[1700000600,"0.25"]]}]}}`, 200, &seen)
	end := time.Unix(1700000600, 0)

	samples, err := c.Range(context.Background(), "prom", "up", end.Add(-10*time.Minute), end, 5*time.Minute)

	require.NoError(t, err)
	require.Equal(t, "/grafana/api/datasources/proxy/uid/prom/api/v1/query_range", seen.Path)
	require.Equal(t, []string{"1700000000", "1700000600", "300"}, []string{seen.Query().Get("start"), seen.Query().Get("end"), seen.Query().Get("step")})
	require.Len(t, samples, 2)
	require.Equal(t, 0.25, samples[1].Value)
	require.Equal(t, time.Unix(1700000600, 0).UTC(), samples[1].At)
}

func Test_Range_FindsNothingInAnEmptyMatrix(t *testing.T) {
	var seen url.URL
	c := server(t, `{"status":"success","data":{"resultType":"matrix","result":[]}}`, 200, &seen)

	samples, err := c.Range(context.Background(), "prom", "up", time.Now().Add(-time.Hour), time.Now(), time.Minute)

	require.NoError(t, err)
	require.Empty(t, samples)
}

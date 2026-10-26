package governor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/governor"
	"github.com/stretchr/testify/require"
)

func prometheusAnswering(t *testing.T, status int, body string) governor.Prometheus {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query", r.URL.Path)
		require.Equal(t, "up * 100", r.URL.Query().Get("query"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return governor.Prometheus{URL: server.URL, Query: "up * 100", Client: server.Client()}
}

func Test_Prometheus_ReadsTheSingleSeries(t *testing.T) {
	p := prometheusAnswering(t, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000.5,"42.25"]}]}}`)

	s, err := p.Read(context.Background())

	require.NoError(t, err)
	require.Equal(t, 42.25, s.Value)
	require.Equal(t, time.Unix(1700000000, 500_000_000), s.At)
}

func Test_Prometheus_RefusesWhatIsNotOneNumber(t *testing.T) {
	answers := map[string]governor.Prometheus{
		"empty":    prometheusAnswering(t, http.StatusOK, `{"status":"success","data":{"result":[]}}`),
		"two":      prometheusAnswering(t, http.StatusOK, `{"status":"success","data":{"result":[{"value":[1,"1"]},{"value":[1,"2"]}]}}`),
		"error":    prometheusAnswering(t, http.StatusBadRequest, `{"status":"error"}`),
		"not json": prometheusAnswering(t, http.StatusOK, `<html>`),
	}
	for name, p := range answers {
		_, err := p.Read(context.Background())
		require.Error(t, err, name)
	}
}

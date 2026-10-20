package pushgateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/repositories/pushgateway"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

type received struct {
	request string
	body    string
}

func recorder(t *testing.T, status int) (*httptest.Server, *received) {
	t.Helper()
	got := &received{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.request, got.body = r.Method+" "+r.URL.Path, string(raw)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func clientFor(srv *httptest.Server) pushgateway.Client {
	return pushgateway.NewClient(config.Config{Metrics: config.Metrics{PushgatewayURL: srv.URL}})
}

func Test_PublishAttachments_PutsOneSeriesPerAttachment(t *testing.T) {
	srv, got := recorder(t, http.StatusOK)

	err := clientFor(srv).PublishAttachments(context.Background(), []entities.Service{{Slug: "api", Databases: []entities.Database{
		{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30},
		{Environment: "dev", Schema: "plateful_dev", User: "kz_plateful_dev", MaxConnections: 10},
	}}, {Slug: "web"}})

	require.NoError(t, err)
	require.Equal(t, "PUT /metrics/job/kalzone_databases", got.request)
	require.Contains(t, got.body, `kalzone_database_attachment{service="api",environment="prod",schema="plateful",mysql_user="kz_plateful"} 30`+"\n")
	require.Contains(t, got.body, `kalzone_database_attachment{service="api",environment="dev",schema="plateful_dev",mysql_user="kz_plateful_dev"} 10`+"\n")
	require.NotContains(t, got.body, `service="web"`)
}

func Test_PublishAttachments_DeletesTheGroupWhenNothingIsAttached(t *testing.T) {
	srv, got := recorder(t, http.StatusAccepted)

	err := clientFor(srv).PublishAttachments(context.Background(), []entities.Service{{Slug: "web"}})

	require.NoError(t, err)
	require.Equal(t, "DELETE /metrics/job/kalzone_databases", got.request)
}

func Test_PublishAttachments_QuotesLabelValues(t *testing.T) {
	srv, got := recorder(t, http.StatusOK)

	err := clientFor(srv).PublishAttachments(context.Background(), []entities.Service{{Slug: "api", Databases: []entities.Database{
		{Environment: "dev", Schema: `a"b\c`, User: "kz_x", MaxConnections: 1},
	}}})

	require.NoError(t, err)
	require.Contains(t, got.body, `schema="a\"b\\c"`)
}

func Test_PublishAttachments_FailsOnAnErrorStatus(t *testing.T) {
	srv, _ := recorder(t, http.StatusInternalServerError)

	err := clientFor(srv).PublishAttachments(context.Background(), []entities.Service{{Slug: "web"}})

	require.Error(t, err)
}

func Test_Enabled_NeedsAURL(t *testing.T) {
	require.False(t, pushgateway.NewClient(config.Config{}).Enabled())
	require.True(t, pushgateway.NewClient(config.Config{Metrics: config.Metrics{PushgatewayURL: "http://pushgateway:9091"}}).Enabled())
}

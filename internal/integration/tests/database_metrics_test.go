package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const attachedShopSeries = `service="places-api",environment="dev",schema="kzt_shop_dev",mysql_user="kz_kzt_shop_dev"`

func Test_AttachedDatabaseShowsOnTheServiceDashboard(t *testing.T) {
	s, p, _ := setupDatabases(t)
	created := createDatabase(t, p.bob, "dev")
	require.Equal(t, http.StatusCreated, created.Status, created.Body)

	attachShop(t, p.bob)
	require.Eventually(t, func() bool {
		return strings.Contains(s.Pushgateway.Body("kalzone_databases"), attachedShopSeries)
	}, 5*time.Second, 50*time.Millisecond)
	require.Contains(t, s.Grafana.DashboardJSON("platform-places-api"), `"title":"Database"`)

	detachShop(t, p.bob)
	require.Eventually(t, func() bool {
		return !strings.Contains(s.Pushgateway.Body("kalzone_databases"), "places-api")
	}, 5*time.Second, 50*time.Millisecond)
	require.NotContains(t, s.Grafana.DashboardJSON("platform-places-api"), `"type":"row"`)
}

func Test_DeletedServiceLeavesNoDatabaseSeriesOrDashboard(t *testing.T) {
	s, p, _ := setupDatabases(t)
	created := createDatabase(t, p.bob, "dev")
	require.Equal(t, http.StatusCreated, created.Status, created.Body)
	attachShop(t, p.bob)
	require.Eventually(t, func() bool {
		return strings.Contains(s.Pushgateway.Body("kalzone_databases"), attachedShopSeries)
	}, 5*time.Second, 50*time.Millisecond)

	require.True(t, startDeleting(t, p.mia, "places-api").Deleted)

	require.Eventually(t, func() bool { return s.Pushgateway.Body("kalzone_databases") == "" }, 5*time.Second, 50*time.Millisecond)
	require.Never(t, func() bool { return s.Grafana.DashboardJSON("platform-places-api") != "" }, 500*time.Millisecond, 50*time.Millisecond)
}

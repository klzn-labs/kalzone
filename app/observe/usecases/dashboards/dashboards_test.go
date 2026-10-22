package dashboards_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/usecases/dashboards"
	"github.com/stretchr/testify/require"
)

type grafana struct {
	enabled bool
	deleted []string
	written []string
}

func (g *grafana) Enabled() bool                           { return g.enabled }
func (g *grafana) EnsureFolder(context.Context, int) error { return nil }
func (g *grafana) UpsertDashboard(_ context.Context, _ int, d map[string]any) error {
	g.written = append(g.written, d["uid"].(string))
	return nil
}
func (g *grafana) DeleteDashboard(_ context.Context, _ int, uid string) error {
	g.deleted = append(g.deleted, uid)
	return nil
}

func Test_Delete_RemovesTheServicesDashboardWhenGrafanaIsSetUp(t *testing.T) {
	on, off := &grafana{enabled: true}, &grafana{}

	require.NoError(t, dashboards.NewDashboardsUsecase(on, &catalog{}, &publisher{}).Delete(context.Background(), "api"))
	require.NoError(t, dashboards.NewDashboardsUsecase(off, &catalog{}, &publisher{}).Delete(context.Background(), "api"))

	require.Equal(t, []string{"platform-api"}, on.deleted)
	require.Empty(t, off.deleted)
}

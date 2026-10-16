package dashboards_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/observe/usecases/dashboards"
	"github.com/stretchr/testify/require"
)

type grafana struct {
	enabled bool
	deleted []string
}

func (g *grafana) Enabled() bool                                         { return g.enabled }
func (g *grafana) EnsureFolder(context.Context) error                    { return nil }
func (g *grafana) UpsertDashboard(context.Context, map[string]any) error { return nil }
func (g *grafana) DeleteDashboard(_ context.Context, uid string) error {
	g.deleted = append(g.deleted, uid)
	return nil
}

func Test_Delete_RemovesTheServicesDashboardWhenGrafanaIsSetUp(t *testing.T) {
	on, off := &grafana{enabled: true}, &grafana{}

	require.NoError(t, dashboards.NewDashboardsUsecase(on).Delete(context.Background(), "api"))
	require.NoError(t, dashboards.NewDashboardsUsecase(off).Delete(context.Background(), "api"))

	require.Equal(t, []string{"platform-api"}, on.deleted)
	require.Empty(t, off.deleted)
}

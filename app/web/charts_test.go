package web_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/web"
	"github.com/stretchr/testify/require"
)

type orgs struct {
	enabled bool
	ids     map[string]int
}

func (o orgs) Enabled() bool { return o.enabled }

func (o orgs) OrgID(name string) (int, bool) {
	id, ok := o.ids[name]
	return id, ok
}

func Test_TeamCharts_EmbedsFourPanelsOfTheTeamsOrgAndLinksTheDashboard(t *testing.T) {
	c := web.QueueCharts(orgs{enabled: true, ids: map[string]int{"team-places": 3}}, "places", "q-1")

	require.True(t, c.Enabled)
	require.Len(t, c.Panels, 4)
	require.Equal(t, "/grafana/d-solo/kalzone-queue?from=now-6h&orgId=3&panelId=1&theme=light&to=now&var-queue_id=q-1", c.Panels[0].Light)
	require.Equal(t, "/grafana/d/kalzone-queue?orgId=3&var-queue_id=q-1", c.Dashboard)
	require.Equal(t, 2, c.Columns)
}

func Test_DatabaseCharts_PutThreeChartsInARowAndTheSlowQueriesUnderThem(t *testing.T) {
	c := web.DatabaseCharts(orgs{enabled: true, ids: map[string]int{"team-places": 3}}, "places", "s", "u")

	require.Equal(t, 3, c.Columns)
	require.Equal(t, []bool{false, false, false, true}, []bool{c.Panels[0].Wide, c.Panels[1].Wide, c.Panels[2].Wide, c.Panels[3].Wide})
	require.Contains(t, c.Panels[3].Light, "panelId=4")
}

func Test_TeamCharts_IsOffWithoutGrafanaOrTheTeamsOrg(t *testing.T) {
	off := web.DatabaseCharts(orgs{enabled: false, ids: map[string]int{"team-places": 3}}, "places", "s", "u")
	unknown := web.DatabaseCharts(orgs{enabled: true}, "places", "s", "u")

	require.False(t, off.Enabled)
	require.False(t, unknown.Enabled)
}

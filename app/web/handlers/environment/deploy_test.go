package environment_test

import (
	"testing"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/web/handlers/environment"
	"github.com/stretchr/testify/require"
)

func rows(n int) []environment.DeploymentRow {
	out := make([]environment.DeploymentRow, n)
	for i := range out {
		out[i] = environment.DeploymentRow{Deployment: deploy.Deployment{Number: n - i}}
	}
	return out
}

func Test_DeployView_ShowsTheFiveNewestDeploymentsFirst(t *testing.T) {
	v := environment.DeployView{Rows: rows(8)}

	require.Len(t, v.RecentRows(), 5)
	require.Equal(t, 8, v.RecentRows()[0].Deployment.Number)
	require.Len(t, v.OlderRows(), 3)
	require.Equal(t, 3, v.OlderRows()[0].Deployment.Number)
}

func Test_DeployView_HasNoOlderDeploymentsWhenFiveOrFewer(t *testing.T) {
	v := environment.DeployView{Rows: rows(3)}

	require.Len(t, v.RecentRows(), 3)
	require.Empty(t, v.OlderRows())
}

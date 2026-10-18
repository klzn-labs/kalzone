package teardown_test

import (
	"context"
	"testing"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/catalog/repositories/teardown"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

type deployments struct{ pending bool }

func (d deployments) Remove(context.Context, actor.Actor, string, string) (deploy.Deployment, error) {
	if d.pending {
		return deploy.Deployment{}, &approval.Pending{ID: "c1", ApproveURL: "https://platform/requests/c1"}
	}
	return deploy.Deployment{ID: "rm1"}, nil
}

func (deployments) Freeze(context.Context, string) error                      { return nil }
func (deployments) Teardown(context.Context, string) (map[string]bool, error) { return nil, nil }
func (deployments) Removing(context.Context, string, string) (bool, error)    { return false, nil }
func (deployments) ForgetRuntime(context.Context, string) error               { return nil }

func Test_Remove_NamesTheRemovalOrTheRequestThatWaitsForApproval(t *testing.T) {
	scheduled, err := teardown.NewHost(deployments{}).Remove(context.Background(), actor.Actor{}, "api", "dev")
	require.NoError(t, err)
	gated, err := teardown.NewHost(deployments{pending: true}).Remove(context.Background(), actor.Actor{}, "api", "prod")
	require.NoError(t, err)

	require.Equal(t, catalog.TeardownStep{Environment: "dev", Status: catalog.TeardownRemoving, DeploymentID: "rm1"}, scheduled)
	require.Equal(t, catalog.TeardownStep{
		Environment: "prod", Status: catalog.TeardownAwaitingApproval, RequestID: "c1", ApproveURL: "https://platform/requests/c1",
	}, gated)
}

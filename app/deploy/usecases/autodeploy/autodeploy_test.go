package autodeploy_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/autodeploy"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type services map[string]entities.ServiceRef

func (s services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	service, ok := s[id]
	if !ok {
		return entities.ServiceRef{}, apperror.NotFound("service not found")
	}
	return service, nil
}

type deployer struct{ deployed []string }

func (d *deployer) AutoDeploy(_ context.Context, serviceID, releaseID string) (entities.Deployment, error) {
	d.deployed = append(d.deployed, serviceID+"@"+releaseID)
	return entities.Deployment{}, nil
}

var catalog = services{
	"s1": {ID: "s1", Slug: "api", AutoDeployDev: true},
	"s2": {ID: "s2", Slug: "worker", AutoDeployDev: false},
}

func released(action, serviceID, trigger string) events.Event {
	return events.Event{
		Action: action, TargetKind: events.TargetRelease, TargetID: serviceID,
		Details: map[string]any{"release_id": "rel-" + serviceID, "trigger": trigger},
	}
}

func Test_Handle_DeploysPushBuiltReleasesOfOptedInServices(t *testing.T) {
	d := &deployer{}
	usecase := autodeploy.NewAutoDeployUsecase(catalog, d)

	require.NoError(t, usecase.Handle(ctx, released("release.built", "s1", "push")))
	require.NoError(t, usecase.Handle(ctx, released("release.shared", "s1", "push")))

	require.Equal(t, []string{"s1@rel-s1", "s1@rel-s1"}, d.deployed)
}

func Test_Handle_IgnoresEverythingElse(t *testing.T) {
	d := &deployer{}
	usecase := autodeploy.NewAutoDeployUsecase(catalog, d)
	ignored := map[string]events.Event{
		"manual build":     released("release.built", "s1", "manual"),
		"registered tag":   released("release.built", "s1", "register"),
		"build started":    released("release.requested", "s1", "push"),
		"failed build":     released("release.failed", "s1", "push"),
		"service opts out": released("release.built", "s2", "push"),
		"not a release":    {Action: "release.built", TargetKind: events.TargetService, TargetID: "s1", Details: map[string]any{"trigger": "push", "release_id": "x"}},
	}

	for name, e := range ignored {
		require.NoError(t, usecase.Handle(ctx, e), name)
	}

	require.Empty(t, d.deployed)
}

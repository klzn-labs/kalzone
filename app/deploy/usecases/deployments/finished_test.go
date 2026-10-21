package deployments_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/stretchr/testify/require"
)

func finishedAt(store *fakestore.Store, id, serviceID, env string, status entities.Status, at time.Time) {
	store.ByID[id] = entities.Deployment{ID: id, ServiceID: serviceID, Environment: env, Status: status, CreatedAt: at, FinishedAt: &at}
}

func Test_FinishedFor_KeepsFinishedDeploysOfVisibleServicesInTheWindow(t *testing.T) {
	store, usecase := setup("")
	now := time.Now().UTC()
	finishedAt(store, "ok", fakestore.API.ID, "prod", entities.StatusSucceeded, now.Add(-time.Hour))
	finishedAt(store, "failed", fakestore.Worker.ID, "prod", entities.StatusFailed, now.Add(-2*time.Hour))
	finishedAt(store, "old", fakestore.API.ID, "prod", entities.StatusSucceeded, now.Add(-30*24*time.Hour))
	finishedAt(store, "dev", fakestore.API.ID, "dev", entities.StatusSucceeded, now)
	finishedAt(store, "gone", "deleted-service", "prod", entities.StatusSucceeded, now)
	store.ByID["running"] = entities.Deployment{ID: "running", ServiceID: fakestore.API.ID, Environment: "prod", Status: entities.StatusInProgress, CreatedAt: now}

	mine, err := usecase.FinishedFor(ctx, ana, "prod", now.Add(-14*24*time.Hour))
	hidden, hiddenErr := usecase.FinishedFor(ctx, outsider, "prod", now.Add(-14*24*time.Hour))

	require.NoError(t, err)
	require.NoError(t, hiddenErr)
	require.ElementsMatch(t, []string{"ok", "failed"}, ids(mine))
	require.Empty(t, hidden)
}

func ids(ds []entities.Deployment) []string {
	result := []string{}
	for _, d := range ds {
		result = append(result, d.ID)
	}
	return result
}

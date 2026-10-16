package tests

import (
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

func hostChange(host string) map[string]any {
	return map[string]any{"spec": map[string]any{"host": host}}
}

func deleteService(t *testing.T, c *integration.Client, slug string) {
	t.Helper()
	resp := c.Do(t, http.MethodDelete, "/api/v1/services/"+slug, nil)
	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
}

func Test_ADeletedServiceGivesUpItsHost(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{"slug": "places-next", "name": "Places next", "owner_team": "places"}, http.StatusCreated, nil)
	require.Equal(t, http.StatusCreated, patchConfig(t, p.bob, "dev", hostChange("places.kalz.one")).Status)
	next := func() integration.Response {
		return p.bob.Send(t, http.MethodPatch, "/api/v1/config/places-next/dev", mergePatch, hostChange("places.kalz.one"))
	}

	whileLive := next()
	deleteService(t, p.mia, "places-api")
	afterDelete := next()

	require.Equal(t, http.StatusConflict, whileLive.Status, whileLive.Body)
	require.Equal(t, http.StatusCreated, afterDelete.Status, afterDelete.Body)
}

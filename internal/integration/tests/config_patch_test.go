package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

const mergePatch = "application/merge-patch+json"

type revisionView struct {
	Number  int               `json:"number"`
	Spec    map[string]any    `json:"spec"`
	Env     map[string]string `json:"env"`
	Message string            `json:"message"`
}

func patchConfig(t *testing.T, c *integration.Client, env string, patch any) integration.Response {
	t.Helper()
	return c.Send(t, http.MethodPatch, "/api/v1/config/places-api/"+env, mergePatch, patch)
}

func Test_AMergePatchChangesOnlyWhatItNames(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"LOG_LEVEL": "info", "OLD_FLAG": "1"}}, http.StatusCreated, nil)
	var rev revisionView

	resp := patchConfig(t, p.bob, "dev", map[string]any{"spec": map[string]any{"replicas": 2}, "env": map[string]any{"LOG_LEVEL": "debug", "OLD_FLAG": nil}, "message": "more replicas"})

	require.Equal(t, http.StatusCreated, resp.Status, resp.Body)
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &rev))
	require.Equal(t, 2, rev.Number)
	require.Equal(t, float64(2), rev.Spec["replicas"])
	require.Equal(t, float64(256), rev.Spec["memory_mb"])
	require.Equal(t, map[string]string{"LOG_LEVEL": "debug"}, rev.Env)
	require.Equal(t, "more replicas", rev.Message)
}

func Test_AMergePatchNeedsItsContentType(t *testing.T) {
	_, p := setupPlaces(t)
	var refused apiError

	p.bob.MustJSON(t, http.MethodPatch, "/api/v1/config/places-api/dev", map[string]any{"env": map[string]string{"A": "1"}}, http.StatusUnsupportedMediaType, &refused)

	require.Equal(t, "err_unsupported_media_type", refused.Error.Code)
	require.Contains(t, refused.Error.Hint, mergePatch)
}

func Test_AMergePatchRefusesWhatTheSpecDoesNotHave(t *testing.T) {
	_, p := setupPlaces(t)
	var unknown, removed apiError

	decode(t, patchConfig(t, p.bob, "dev", map[string]any{"spec": map[string]any{"replica": 2}}).Body, &unknown)
	decode(t, patchConfig(t, p.bob, "dev", map[string]any{"spec": nil}).Body, &removed)

	require.Equal(t, "spec.replica", unknown.Error.Field)
	require.Contains(t, unknown.Error.Hint, "replicas")
	require.Equal(t, "spec", removed.Error.Field)
}

func Test_AMergePatchOnProdBecomesAChangeRequest(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	var pending pendingChange

	resp := patchConfig(t, agent, "prod", map[string]any{"env": map[string]string{"LOG_LEVEL": "debug"}})

	require.Equal(t, http.StatusAccepted, resp.Status, resp.Body)
	decode(t, resp.Body, &pending)
	require.Equal(t, "change_request", pending.Kind)
	require.Equal(t, "pending", poll(t, agent, pending).Status)
}

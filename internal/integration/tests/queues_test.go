package tests

import (
	"net/http"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

type queueDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Consumer   string   `json:"consumer"`
	Publishers []string `json:"publishers"`
	Version    int      `json:"version"`
	Paused     bool     `json:"paused"`
	Settings   struct {
		MaxAttempts     int    `json:"max_attempts"`
		DeliveryTimeout string `json:"delivery_timeout"`
	} `json:"settings"`
}

func setupQueue(t *testing.T) (*integration.Suite, people) {
	t.Helper()
	s, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "places-worker", "name": "Places worker", "owner_team": "places", "image_repository": "docker.io/adrianozdp/places-api",
	}, http.StatusCreated, nil)
	return s, p
}

func createQueue(t *testing.T, c *integration.Client, env string, status int) queueDTO {
	t.Helper()
	var q queueDTO
	c.MustJSON(t, http.MethodPost, "/api/v1/queues", map[string]any{
		"name": "place-import", "environment": env, "consumer": "places-worker", "path": "/hooks/place-import",
		"publishers": []string{"places-api"},
	}, status, &q)
	return q
}

func dropPublishers(t *testing.T, c *integration.Client) {
	t.Helper()
	dropped := c.Do(t, http.MethodPatch, "/api/v1/queues/dev/place-import", map[string]any{"publishers": []string{}, "version": 1})
	require.Equal(t, http.StatusOK, dropped.Status, dropped.Body)
}

func Test_QueuesAreCreatedWithDefaultsAndEditedByPolicy(t *testing.T) {
	_, p := setupQueue(t)

	created := createQueue(t, p.bob, "dev", http.StatusCreated)
	bobProd := p.bob.Do(t, http.MethodPost, "/api/v1/queues", map[string]any{"name": "place-import", "environment": "prod", "consumer": "places-worker", "path": "/hooks/place-import"})
	updated := p.bob.Do(t, http.MethodPatch, "/api/v1/queues/dev/place-import", map[string]any{
		"settings": map[string]any{"path": "/hooks/place-import", "rate_per_second": 5, "max_in_flight": 2, "max_attempts": 9,
			"backoff": map[string]string{"kind": "fixed", "initial": "2s", "max": "2s"}, "delivery_timeout": "10s",
			"retention": "24h", "max_message_bytes": 65536},
		"message": "slower", "version": 1,
	})
	eve := p.eve.Do(t, http.MethodPatch, "/api/v1/queues/dev/place-import", map[string]any{"message": "x", "version": 2})

	require.Equal(t, "places-worker", created.Consumer)
	require.ElementsMatch(t, []string{"places-worker", "places-api"}, created.Publishers)
	require.Equal(t, 5, created.Settings.MaxAttempts)
	require.Equal(t, "30s", created.Settings.DeliveryTimeout)
	require.Equal(t, http.StatusForbidden, bobProd.Status, bobProd.Body)
	require.Equal(t, http.StatusOK, updated.Status, updated.Body)
	require.Contains(t, updated.Body, `"version":2`)
	require.Equal(t, http.StatusForbidden, eve.Status, eve.Body)
}

func Test_AnEditBasedOnAnOlderVersionIsAConflict(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	path := "/api/v1/queues/dev/place-import"

	first := p.bob.Do(t, http.MethodPatch, path, map[string]any{"publishers": []string{}, "version": 1})
	second := p.bob.Do(t, http.MethodPatch, path, map[string]any{"publishers": []string{"places-api"}, "version": 1})
	missing := p.bob.Do(t, http.MethodPatch, path, map[string]any{"publishers": []string{}})

	require.Equal(t, http.StatusOK, first.Status, first.Body)
	require.Equal(t, http.StatusConflict, second.Status, second.Body)
	require.Equal(t, http.StatusBadRequest, missing.Status, missing.Body)
}

type queueStats struct {
	Depth int64 `json:"depth"`
	Fresh bool  `json:"fresh"`
}

func devQueueStats(t *testing.T, c *integration.Client) queueStats {
	t.Helper()
	var stats queueStats
	c.MustJSON(t, http.MethodGet, "/api/v1/queues/dev/place-import/stats", nil, http.StatusOK, &stats)
	return stats
}

func Test_TheGatewaySeesDesiredQueuesAndReportsStats(t *testing.T) {
	s, p := setupQueue(t)
	empty := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)

	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	reported := s.Queued(t, http.MethodPost, "/queued/v1/stats", map[string]any{"stats": []map[string]any{{"queue_id": q.ID, "depth": 7}}})
	anonymous := s.Raw(t, http.MethodGet, "/queued/v1/desired", nil, nil)
	stats := devQueueStats(t, p.bob)

	require.Equal(t, http.StatusOK, empty.Status, empty.Body)
	require.Contains(t, empty.Body, `"queues":[]`)
	require.Contains(t, desired.Body, `"target":"http://places-worker-dev:8080/hooks/place-import"`)
	require.Contains(t, desired.Body, `"signing_key":"`)
	require.Equal(t, http.StatusNoContent, reported.Status, reported.Body)
	require.Equal(t, http.StatusUnauthorized, anonymous.Status)
	require.EqualValues(t, 7, stats.Depth)
	require.True(t, stats.Fresh)
}

func Test_DeleteNeedsAnEmptyQueueOrForce(t *testing.T) {
	s, p := setupQueue(t)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)
	s.Queued(t, http.MethodPost, "/queued/v1/stats", map[string]any{"stats": []map[string]any{{"queue_id": q.ID, "depth": 2}}})
	dropPublishers(t, p.bob)

	held := p.bob.Do(t, http.MethodDelete, "/api/v1/queues/dev/place-import", nil)
	forced := p.bob.Do(t, http.MethodDelete, "/api/v1/queues/dev/place-import?force=true", nil)
	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)

	require.Equal(t, http.StatusConflict, held.Status, held.Body)
	require.Equal(t, http.StatusNoContent, forced.Status, forced.Body)
	require.Contains(t, desired.Body, `"kind":"delete"`)
}

func Test_DeadLetterCommandsReachTheGateway(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	replay := p.bob.Do(t, http.MethodPost, "/api/v1/queues/dev/place-import/dead/m1/replay", nil)
	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	var parsed struct {
		Commands []struct{ ID, Kind, Argument string } `json:"commands"`
	}
	decode(t, desired.Body, &parsed)
	done := s.Queued(t, http.MethodPost, "/queued/v1/commands/"+parsed.Commands[0].ID+"/done", nil)
	after := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)

	require.Equal(t, http.StatusAccepted, replay.Status, replay.Body)
	require.Equal(t, "replay", parsed.Commands[0].Kind)
	require.Equal(t, "m1", parsed.Commands[0].Argument)
	require.Equal(t, http.StatusNoContent, done.Status)
	require.Contains(t, after.Body, `"commands":[]`)
}

func Test_ReservedQueueNamesCannotBeSetByHand(t *testing.T) {
	_, p := setupQueue(t)

	resp := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-worker/dev/revisions", map[string]any{"set_env": map[string]string{"PLATFORM_QUEUE_TOKEN": "x"}})

	require.Equal(t, http.StatusBadRequest, resp.Status, resp.Body)
	require.Contains(t, resp.Body, "set by the platform")
}

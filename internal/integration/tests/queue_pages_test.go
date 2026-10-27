package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func queueForm(csrf string) url.Values {
	return url.Values{
		"_csrf": {csrf}, "name": {"other"}, "environment": {"dev"}, "consumer": {"places-worker"}, "path": {"/hooks/other"},
		"publishers": {"places-api"}, "rate_per_second": {"5"}, "max_in_flight": {"2"}, "max_attempts": {"4"},
		"backoff_kind": {"fixed"}, "backoff_initial": {"2s"}, "backoff_max": {"2s"}, "delivery_timeout": {"30s"},
		"retention": {"24h"}, "max_message_bytes": {"65536"},
	}
}

func Test_TheQueuePagesListAndCreate(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	list := p.mia.Do(t, http.MethodGet, "/resources/queues", nil)
	form := p.bob.Do(t, http.MethodGet, "/resources/new/queue", nil)
	created := p.bob.Form(t, "/resources/queues", queueForm(p.bob.CSRFToken))
	settings := p.mia.Do(t, http.MethodGet, "/resources/queues/dev/other?tab=settings", nil)

	require.Equal(t, http.StatusOK, list.Status, list.Body)
	require.Contains(t, list.Body, "place-import")
	require.Contains(t, list.Body, "New queue")
	require.Equal(t, http.StatusOK, form.Status, form.Body)
	require.Equal(t, http.StatusNoContent, created.Status, created.Body)
	require.Contains(t, settings.Body, "5 / s")
	require.Contains(t, settings.Body, "/hooks/other")
}

func Test_TheQueuePageShowsStateAndPauses(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	base := "/resources/queues/dev/place-import"

	detail := p.mia.Do(t, http.MethodGet, base, nil)
	paused := p.bob.Form(t, base+"/pause", url.Values{"_csrf": {p.bob.CSRFToken}})
	afterPause := p.mia.Do(t, http.MethodGet, base, nil)

	require.Equal(t, http.StatusOK, detail.Status, detail.Body)
	require.Contains(t, detail.Body, "Dead letters")
	require.Contains(t, detail.Body, "No data")
	require.Equal(t, http.StatusNoContent, paused.Status, paused.Body)
	require.Contains(t, afterPause.Body, "Paused")
	require.Contains(t, afterPause.Body, "Resume delivery")
}

func Test_TheQueuePageReplaysAndDeletes(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	dropPublishers(t, p.bob)
	base := "/resources/queues/dev/place-import"

	replay := p.bob.Form(t, base+"/dead/replay-all", url.Values{"_csrf": {p.bob.CSRFToken}})
	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	deleted := p.bob.Form(t, base+"/delete", url.Values{"_csrf": {p.bob.CSRFToken}, "force": {"on"}})
	list := p.mia.Do(t, http.MethodGet, "/resources/queues", nil)

	require.Equal(t, http.StatusNoContent, replay.Status, replay.Body)
	require.Contains(t, desired.Body, `"kind":"replay_all"`)
	require.Equal(t, http.StatusNoContent, deleted.Status, deleted.Body)
	require.NotContains(t, list.Body, "place-import")
}

func Test_TheQueuePageSavesAdaptiveSettingsAndShowsTheRate(t *testing.T) {
	s, p := setupQueue(t)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)
	form := queueForm(p.bob.CSRFToken)
	form.Set("version", "1")
	form.Set("adaptive", "on")
	form.Set("min_rate_per_second", "0.5")
	adaptive := map[string]any{"factor": 0.4, "database_cpu": 62.5, "target_cpu": 50, "fail_safe": false}

	saved := p.bob.Form(t, "/resources/queues/dev/place-import/settings", form)
	s.Queued(t, http.MethodPost, "/queued/v1/stats", map[string]any{"stats": []map[string]any{
		{"queue_id": q.ID, "effective_rate_per_second": 2.3, "adaptive": adaptive},
	}})
	page := p.mia.Do(t, http.MethodGet, "/resources/queues/dev/place-import?tab=settings", nil)

	require.Equal(t, http.StatusNoContent, saved.Status, saved.Body)
	require.Contains(t, page.Body, "Adaptive · slowing down")
	require.Contains(t, page.Body, "2.3 / s")
	require.Contains(t, page.Body, "DB CPU 62.5% of a core, target 50% · never below 0.5 / s")
	require.Contains(t, page.Body, "5 / s, adaptive down to 0.5 / s, 2 in flight")
	require.Contains(t, page.Body, `name="adaptive" checked`)
}

func Test_TheQueuePageSavesSettingsAsANewVersion(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	form := queueForm(p.bob.CSRFToken)
	form.Set("version", "1")
	form.Set("message", "slower for the quota")

	saved := p.bob.Form(t, "/resources/queues/dev/place-import/settings", form)
	stale := p.bob.Form(t, "/resources/queues/dev/place-import/settings", form)
	activity := p.mia.Do(t, http.MethodGet, "/resources/queues/dev/place-import?tab=activity", nil)

	require.Equal(t, http.StatusNoContent, saved.Status, saved.Body)
	require.NotEqual(t, http.StatusNoContent, stale.Status, stale.Body)
	require.Contains(t, activity.Body, "slower for the quota")
	require.Contains(t, activity.Body, "5 / s, 2 in flight, 4 attempts")
}

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ResourcesPageListsQueues(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	page := p.mia.Do(t, http.MethodGet, "/resources", nil)
	prod := p.mia.Do(t, http.MethodGet, "/resources?environment=prod", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, "place-import")
	require.Contains(t, page.Body, "places-api")
	require.Contains(t, page.Body, "New queue")
	require.Regexp(t, `nav-link active"[^>]*href="/resources"`, page.Body)
	require.NotContains(t, prod.Body, "place-import")
}

func Test_ResourcesPageRowsCarryFullRowLink(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	page := p.mia.Do(t, http.MethodGet, "/resources", nil)

	require.Regexp(t, `<tr class="row-linked">\s*<td><a class="mono row-link" href="/resources/queues/dev/place-import">`, page.Body)
}

func Test_ResourcesPageFiltersByKind(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	queues := p.mia.Do(t, http.MethodGet, "/resources?kind=queue", nil)
	unknown := p.mia.Do(t, http.MethodGet, "/resources?kind=database", nil)

	require.Contains(t, queues.Body, "place-import")
	require.NotContains(t, unknown.Body, "place-import")
}

func Test_OldQueueURLsRedirect(t *testing.T) {
	_, p := setupQueue(t)
	cases := map[string]string{
		"/queues":                           "/resources?kind=queue",
		"/queues/":                          "/resources?kind=queue",
		"/queues?environment=dev":           "/resources?kind=queue&environment=dev",
		"/queues/new":                       "/resources/new/queue",
		"/resources/new":                    "/resources/new/queue",
		"/queues/dev/place-import?tab=dead": "/resources/queues/dev/place-import?tab=dead",
	}
	for from, to := range cases {
		res := p.mia.Do(t, http.MethodGet, from, nil)
		require.Equal(t, http.StatusMovedPermanently, res.Status, from)
		require.Equal(t, to, res.Header.Get("Location"), from)
	}
}

func Test_QueueDeleteBlockedShowsWhy(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	blocked := p.mia.Do(t, http.MethodGet, "/resources/queues/dev/place-import?tab=settings", nil)
	dropPublishers(t, p.bob)
	free := p.mia.Do(t, http.MethodGet, "/resources/queues/dev/place-import?tab=settings", nil)

	require.Contains(t, blocked.Body, "Remove these publishers first: places-api")
	require.Regexp(t, `<button class="btn btn-danger" disabled`, blocked.Body)
	require.NotContains(t, free.Body, "Remove these publishers first")
}

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ResourcesPageIsACatalogWithWhatIsInUse(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	page := p.mia.Do(t, http.MethodGet, "/resources", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, "Background work")
	require.Contains(t, page.Body, "<b>1 in use</b><span>dev 1</span>")
	require.Contains(t, page.Body, `href="/resources/new/queue"`)
	require.Contains(t, page.Body, `href="/resources/crons"`)
	require.NotContains(t, page.Body, "place-import")
	require.Regexp(t, `nav-link active"[^>]*href="/resources"`, page.Body)
}

func Test_EachResourceKindHasItsOwnList(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	queues := p.mia.Do(t, http.MethodGet, "/resources/queues", nil)
	prod := p.mia.Do(t, http.MethodGet, "/resources/queues?environment=prod", nil)
	databases := p.mia.Do(t, http.MethodGet, "/resources/databases", nil)

	require.Equal(t, http.StatusOK, queues.Status, queues.Body)
	require.Contains(t, queues.Body, "places-api")
	require.Contains(t, queues.Body, "New queue")
	require.Regexp(t, `nav-link active"[^>]*href="/resources/queues"`, queues.Body)
	require.Regexp(t, `<tr class="row-linked">\s*<td><a class="mono row-link" href="/resources/queues/dev/place-import">`, queues.Body)
	require.NotContains(t, prod.Body, "place-import")
	require.Equal(t, http.StatusOK, databases.Status, databases.Body)
	require.NotContains(t, databases.Body, "place-import")
}

func Test_TheCronListShowsScheduleAndService(t *testing.T) {
	_, p := setupPlaces(t)
	empty := p.mia.Do(t, http.MethodGet, "/resources/crons", nil)
	createCron(t, p.bob, "dev")

	list := p.mia.Do(t, http.MethodGet, "/resources/crons", nil)

	require.Contains(t, empty.Body, "No crons yet")
	require.Equal(t, http.StatusOK, list.Status, list.Body)
	require.Contains(t, list.Body, "nightly-import")
	require.Contains(t, list.Body, "0 3 * * *")
	require.Contains(t, list.Body, "places-api")
	require.Regexp(t, `nav-link active"[^>]*href="/resources/crons"`, list.Body)
}

func Test_SidebarCountsArriveOutOfBand(t *testing.T) {
	_, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	createCron(t, p.bob, "dev")

	counts := p.mia.Do(t, http.MethodGet, "/resources/counts", nil)

	require.Equal(t, http.StatusOK, counts.Status, counts.Body)
	require.Contains(t, counts.Body, `id="nav-count-queue" hx-swap-oob="true">1<`)
	require.Contains(t, counts.Body, `id="nav-count-database" hx-swap-oob="true">0<`)
	require.Contains(t, counts.Body, `id="nav-count-cron" hx-swap-oob="true">1<`)
}

func Test_OldKindLinksRedirectToTheKindsList(t *testing.T) {
	_, p := setupQueue(t)
	cases := map[string]string{
		"/resources?kind=queue":                    "/resources/queues",
		"/resources?kind=database&environment=dev": "/resources/databases?environment=dev",
		"/resources?kind=cron":                     "/resources/crons",
	}
	for from, to := range cases {
		res := p.mia.Do(t, http.MethodGet, from, nil)
		require.Equal(t, http.StatusMovedPermanently, res.Status, from)
		require.Equal(t, to, res.Header.Get("Location"), from)
	}
}

func Test_OldQueueURLsRedirect(t *testing.T) {
	_, p := setupQueue(t)
	cases := map[string]string{
		"/queues":                           "/resources/queues",
		"/queues/":                          "/resources/queues",
		"/queues?environment=dev":           "/resources/queues?environment=dev",
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

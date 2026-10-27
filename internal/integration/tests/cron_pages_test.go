package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

const cronWebPage = "/resources/crons/dev/nightly-import"

func cronSettingsForm(csrf string) url.Values {
	return url.Values{"_csrf": {csrf}, "schedule": {"@hourly"}, "path": {"/hooks/hourly"}, "timeout": {"5m"}, "payload": {`{"full": true}`}}
}

func getCron(t *testing.T, p people) cronDTO {
	t.Helper()
	var c cronDTO
	p.bob.MustJSON(t, http.MethodGet, cronPath, nil, http.StatusOK, &c)
	return c
}

func Test_TheCronListLinksEachRow(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")

	list := p.mia.Do(t, http.MethodGet, "/resources/crons", nil)

	require.Equal(t, http.StatusOK, list.Status, list.Body)
	require.Regexp(t, `<tr class="row-linked">\s*<td><a class="mono row-link" href="/resources/crons/dev/nightly-import">`, list.Body)
}

func Test_TheCronPageShowsSettingsAndRuns(t *testing.T) {
	s, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	reportRuns(t, s, gatewayCrons(t, s).Crons[0].ID, 2)

	page := p.bob.Do(t, http.MethodGet, cronWebPage, nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, `<span class="mono">0 3 * * *</span>`)
	require.Contains(t, page.Body, "places-api → /hooks/import")
	require.Contains(t, page.Body, "Recent runs")
	require.Contains(t, page.Body, "succeeded")
	require.Contains(t, page.Body, "Change settings")
	require.Contains(t, page.Body, "Run now")
	require.Regexp(t, `nav-link active"[^>]*href="/resources/crons"`, page.Body)
}

func Test_TheCronFormCreatesACron(t *testing.T) {
	_, p := setupPlaces(t)
	form := url.Values{"_csrf": {p.bob.CSRFToken}, "name": {"nightly-import"}, "environment": {"dev"}, "service": {"places-api"},
		"schedule": {"0 3 * * *"}, "path": {"/hooks/import"}, "timeout": {"2m"}, "payload": {`{"city": "lisbon"}`}}

	page := p.bob.Do(t, http.MethodGet, "/resources/new/cron", nil)
	created := p.bob.Form(t, "/resources/crons", form)
	form.Set("schedule", "@every 1m")
	form.Set("name", "other")
	refused := p.bob.Form(t, "/resources/crons", form)

	require.Contains(t, page.Body, `<option value="places-api">`)
	require.Equal(t, http.StatusNoContent, created.Status, created.Body)
	require.Equal(t, http.StatusUnprocessableEntity, refused.Status, refused.Body)
	c := getCron(t, p)
	require.Equal(t, "2m0s", c.Timeout)
	require.Equal(t, map[string]any{"city": "lisbon"}, c.Payload)
}

func Test_TheCronPageSavesSettings(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")

	saved := p.bob.Form(t, cronWebPage+"/settings", cronSettingsForm(p.bob.CSRFToken))

	require.Equal(t, http.StatusNoContent, saved.Status, saved.Body)
	c := getCron(t, p)
	require.Equal(t, "@hourly", c.Schedule)
	require.Equal(t, "/hooks/hourly", c.Path)
	require.Equal(t, "5m0s", c.Timeout)
	require.Equal(t, map[string]any{"full": true}, c.Payload)
}

func Test_TheCronPagePausesAndResumes(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	csrf := url.Values{"_csrf": {p.bob.CSRFToken}}

	paused := p.bob.Form(t, cronWebPage+"/pause", csrf)
	whilePaused := getCron(t, p)
	pausedPage := p.bob.Do(t, http.MethodGet, cronWebPage, nil)
	resumed := p.bob.Form(t, cronWebPage+"/resume", csrf)

	require.Equal(t, http.StatusNoContent, paused.Status, paused.Body)
	require.True(t, whilePaused.Paused)
	require.Contains(t, pausedPage.Body, "none while paused")
	require.Equal(t, http.StatusNoContent, resumed.Status, resumed.Body)
	require.False(t, getCron(t, p).Paused)
}

func Test_RunNowFromTheCronPageQueuesACommand(t *testing.T) {
	s, p := setupPlaces(t)
	createCron(t, p.bob, "dev")

	run := p.bob.Form(t, cronWebPage+"/run", url.Values{"_csrf": {p.bob.CSRFToken}})

	require.Equal(t, http.StatusNoContent, run.Status, run.Body)
	desired := gatewayCrons(t, s)
	require.Len(t, desired.Commands, 1)
	require.Equal(t, desired.Crons[0].ID, desired.Commands[0].CronID)
}

func Test_TheCronPageDeletes(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")

	deleted := p.bob.Form(t, cronWebPage+"/delete", url.Values{"_csrf": {p.bob.CSRFToken}})
	gone := p.bob.Do(t, http.MethodGet, cronWebPage, nil)

	require.Equal(t, http.StatusNoContent, deleted.Status, deleted.Body)
	require.Equal(t, http.StatusNotFound, gone.Status, gone.Body)
	p.bob.MustJSON(t, http.MethodGet, cronPath, nil, http.StatusNotFound, nil)
}

func Test_ReadersDoNotSeeCronEditControls(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.mia, "prod")
	page := "/resources/crons/prod/nightly-import"

	member := p.bob.Do(t, http.MethodGet, page, nil)
	maintainer := p.mia.Do(t, http.MethodGet, page, nil)

	require.Equal(t, http.StatusOK, member.Status, member.Body)
	require.NotContains(t, member.Body, "Change settings")
	require.NotContains(t, member.Body, "Run now")
	require.NotContains(t, member.Body, "Delete cron")
	require.Contains(t, maintainer.Body, "Change settings")
	require.Contains(t, maintainer.Body, "Run now")
}

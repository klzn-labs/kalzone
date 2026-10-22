package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const databasePage = "/resources/databases/dev/" + testDatabase

func databaseForm(csrf string) url.Values {
	return url.Values{"_csrf": {csrf}, "name": {testDatabase}, "environment": {"dev"}, "team": {"places"}, "max_connections": {"7"}}
}

func Test_DatabasePageShowsFactsNotPassword(t *testing.T) {
	_, p, _ := setupDatabases(t)
	created := p.bob.Form(t, "/resources/databases", databaseForm(p.bob.CSRFToken))
	attachShop(t, p.bob)

	page := p.mia.Do(t, http.MethodGet, databasePage, nil)

	require.Equal(t, http.StatusNoContent, created.Status, created.Body)
	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, devSchema)
	require.Contains(t, page.Body, "kz_"+devSchema)
	require.Contains(t, page.Body, "places-api")
	require.Contains(t, page.Body, "up to 7")
	require.Contains(t, page.Body, "Attach and detach take effect on each service")
	require.Contains(t, page.Body, `hx-confirm="Detach places-api?`)
	require.NotContains(t, strings.ToLower(page.Body), "password")
}

func Test_DatabaseDeleteDisabledWhileAttached(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)
	attachShop(t, p.bob)

	page := p.mia.Do(t, http.MethodGet, databasePage, nil)

	require.Contains(t, page.Body, "Detach these first: places-api.")
	require.Contains(t, page.Body, `<button class="btn btn-danger" disabled>`)
}

func Test_DatabasePageAttachesDetachesAndDeletes(t *testing.T) {
	_, p, admin := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)
	csrf := url.Values{"_csrf": {p.bob.CSRFToken}}

	before := p.bob.Do(t, http.MethodGet, databasePage, nil)
	attached := p.bob.Form(t, databasePage+"/attach", url.Values{"_csrf": {p.bob.CSRFToken}, "service": {"places-api"}})
	detached := p.bob.Form(t, databasePage+"/detach/places-api", csrf)
	deleted := p.bob.Form(t, databasePage+"/delete", url.Values{"_csrf": {p.bob.CSRFToken}, "confirm": {testDatabase}})

	require.Contains(t, before.Body, `<option value="places-api">`)
	require.Equal(t, http.StatusNoContent, attached.Status, attached.Body)
	require.Equal(t, http.StatusNoContent, detached.Status, detached.Body)
	require.Equal(t, http.StatusNoContent, deleted.Status, deleted.Body)
	require.False(t, schemaExists(t, admin, devSchema))
}

func Test_DatabaseListsInResources(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)
	attachShop(t, p.bob)

	list := p.mia.Do(t, http.MethodGet, "/resources?kind=database", nil)
	queues := p.mia.Do(t, http.MethodGet, "/resources?kind=queue", nil)
	form := p.bob.Do(t, http.MethodGet, "/resources/new/database", nil)

	require.Contains(t, list.Body, "New database")
	require.Contains(t, list.Body, testDatabase)
	require.Contains(t, list.Body, devSchema)
	require.Contains(t, list.Body, "places-api")
	require.NotContains(t, queues.Body, testDatabase)
	require.Equal(t, http.StatusOK, form.Status, form.Body)
	require.Contains(t, form.Body, "Create database")
}

func Test_ServicePageListsAttachedResources(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)
	attachShop(t, p.bob)

	page := p.mia.Do(t, http.MethodGet, "/services/places-api", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, "Manage in Resources")
	require.Contains(t, page.Body, databasePage)
	require.Contains(t, page.Body, "PLATFORM_DB_* on deploy")
}

func Test_DatabasePageShowsDrainingServiceAfterDetach(t *testing.T) {
	s, p, _ := setupDatabases(t)
	liveWithDatabase(t, s, p, builtRelease(t, s, p))
	detachShop(t, p.bob)

	page := p.mia.Do(t, http.MethodGet, databasePage, nil)

	require.Contains(t, page.Body, "Detached, still running with these credentials")
	require.Contains(t, page.Body, "Redeploy to finish")
	require.Contains(t, page.Body, "Redeploy or remove: places-api.")
	require.NotContains(t, page.Body, "Detach these first")
	require.Contains(t, page.Body, `<button class="btn btn-danger" disabled>`)
}

func Test_DatabaseDeleteEnabledWhenNothingIsAttached(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)

	page := p.mia.Do(t, http.MethodGet, databasePage, nil)

	require.Contains(t, page.Body, `<button class="btn btn-danger">`)
	require.Contains(t, page.Body, `name="confirm"`)
	require.Contains(t, page.Body, `placeholder="`+testDatabase+`"`)
	require.NotContains(t, page.Body, "Detach these first")
}

func Test_DatabaseDeleteRefusesAWrongTypedName(t *testing.T) {
	_, p, admin := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)

	missing := p.bob.Form(t, databasePage+"/delete", url.Values{"_csrf": {p.bob.CSRFToken}})
	wrong := p.bob.Form(t, databasePage+"/delete", url.Values{"_csrf": {p.bob.CSRFToken}, "confirm": {"other"}})

	require.Equal(t, http.StatusUnprocessableEntity, missing.Status, missing.Body)
	require.Equal(t, http.StatusUnprocessableEntity, wrong.Status, wrong.Body)
	require.Contains(t, wrong.Body, "type the database name to confirm")
	require.True(t, schemaExists(t, admin, devSchema))
	require.Equal(t, http.StatusOK, p.bob.Do(t, http.MethodGet, "/api/v1/databases/dev/"+testDatabase, nil).Status)
}

func Test_DatabasePageHidesControlsFromViewersWhoCannotEdit(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.mia, "prod").Status)
	prodPage := "/resources/databases/prod/" + testDatabase

	page := p.bob.Do(t, http.MethodGet, prodPage, nil)
	maintainer := p.mia.Do(t, http.MethodGet, prodPage, nil)
	outsider := p.eve.Do(t, http.MethodGet, prodPage, nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.NotContains(t, page.Body, "/attach")
	require.NotContains(t, page.Body, "/delete")
	require.NotContains(t, page.Body, "Delete this database")
	require.Contains(t, maintainer.Body, "Delete this database")
	require.Equal(t, http.StatusNotFound, outsider.Status)
}

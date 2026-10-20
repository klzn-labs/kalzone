package tests

import (
	"database/sql"
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

const oldPage = "/resources/databases/dev/" + oldDatabase

func adoptedPage(t *testing.T) (string, *adoptedFixture) {
	t.Helper()
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	playRootStep(t, admin)
	releaseID := builtRelease(t, s, p)
	require.Equal(t, http.StatusCreated, adoptOld(t, s, p).Status)
	return releaseID, &adoptedFixture{p: p, s: s, admin: admin}
}

func Test_DatabasePageLabelsAnAdoptedDatabase(t *testing.T) {
	_, f := adoptedPage(t)
	f.p.bob.MustJSON(t, http.MethodDelete, oldPath+"/attachments/places-api", nil, http.StatusOK, nil)

	page := f.p.bob.Do(t, http.MethodGet, oldPage, nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, `<span class="badge">Adopted</span>`)
	require.Contains(t, page.Body, "Release this database")
	require.Contains(t, page.Body, "Removes the platform's user and record. The schema <span class=\"mono\">"+oldSchema+"</span> and its data stay in MySQL.")
	require.Contains(t, page.Body, "Release "+oldDatabase+"</button>")
	require.Contains(t, page.Body, `name="confirm"`)
	require.NotContains(t, page.Body, "Delete this database")
}

func Test_DatabaseReleaseKeepsTheSchema(t *testing.T) {
	releaseID, f := adoptedPage(t)
	f.p.bob.MustJSON(t, http.MethodDelete, oldPath+"/attachments/places-api", nil, http.StatusOK, nil)
	deployPlaces(t, f.p.bob, releaseID)
	runToSuccess(t, f.s)

	released := f.p.bob.Form(t, oldPage+"/delete", url.Values{"_csrf": {f.p.bob.CSRFToken}, "confirm": {oldDatabase}})
	list := f.p.bob.Do(t, http.MethodGet, "/resources?kind=database", nil)
	gone := f.p.bob.Do(t, http.MethodGet, oldPage, nil)

	require.Equal(t, http.StatusNoContent, released.Status, released.Body)
	require.Equal(t, http.StatusNotFound, gone.Status, gone.Body)
	f.p.root.MustJSON(t, http.MethodGet, oldPath, nil, http.StatusNotFound, nil)
	require.Contains(t, list.Body, "Released "+oldDatabase+"; its data stays in MySQL.")
	require.Equal(t, "lisbon", placeAsRoot(t, f.admin))
	require.False(t, userExists(t, f.admin, oldUser))
}

func Test_ResourcesListMarksAdoptedRows(t *testing.T) {
	_, f := adoptedPage(t)

	list := f.p.mia.Do(t, http.MethodGet, "/resources?kind=database", nil)

	require.Contains(t, list.Body, oldDatabase)
	require.Contains(t, list.Body, `<span class="badge">Adopted</span>`)
}

type adoptedFixture struct {
	p     people
	s     *integration.Suite
	admin *sql.DB
}

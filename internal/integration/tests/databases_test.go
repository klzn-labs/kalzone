package tests

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

const (
	testDatabase = "kzt-shop"
	devSchema    = "kzt_shop_dev"
	prodSchema   = "kzt_shop"
	attachNote   = "takes effect on each service's next deploy"
	databasePath = "/api/v1/databases/dev/" + testDatabase
)

type attachmentDTO struct {
	Service  string `json:"service"`
	Draining bool   `json:"draining"`
}

type databaseDTO struct {
	Name           string          `json:"name"`
	Environment    string          `json:"environment"`
	Host           string          `json:"host"`
	Port           int             `json:"port"`
	Schema         string          `json:"schema"`
	Username       string          `json:"username"`
	MaxConnections int             `json:"max_connections"`
	Attachments    []attachmentDTO `json:"attachments"`
	Note           string          `json:"note"`
}

func adminConnection(t *testing.T) *sql.DB {
	t.Helper()
	c, err := driver.ParseDSN(testdb.DSN())
	require.NoError(t, err)
	c.DBName = ""
	admin, err := sql.Open("mysql", c.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	return admin
}

func setupDatabases(t *testing.T) (*integration.Suite, people, *sql.DB) {
	t.Helper()
	s, p := setupPlaces(t)
	admin := adminConnection(t)
	t.Cleanup(func() { dropScratch(admin) })
	return s, p, admin
}

func dropScratch(admin *sql.DB) {
	for _, schema := range []string{devSchema, prodSchema} {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`")
		_, _ = admin.Exec("DROP USER IF EXISTS 'kz_" + schema + "'@'%'")
	}
}

func schemaExists(t *testing.T, admin *sql.DB, schema string) bool {
	t.Helper()
	var found int
	row := admin.QueryRow("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", schema)
	require.NoError(t, row.Scan(&found))
	return found > 0
}

func createDatabase(t *testing.T, c *integration.Client, env string) integration.Response {
	t.Helper()
	return c.Do(t, http.MethodPost, "/api/v1/databases", map[string]any{"name": testDatabase, "environment": env, "team": "places"})
}

func attachShop(t *testing.T, c *integration.Client) databaseDTO {
	t.Helper()
	var d databaseDTO
	c.MustJSON(t, http.MethodPost, databasePath+"/attachments", map[string]string{"service": "places-api"}, http.StatusOK, &d)
	return d
}

func deployPlaces(t *testing.T, c *integration.Client, releaseID string) {
	t.Helper()
	c.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusAccepted, nil)
}

func requireWorkingDSN(t *testing.T, dsn string) {
	t.Helper()
	conn, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.ExecContext(context.Background(), "CREATE TABLE things (id INT PRIMARY KEY)")
	require.NoError(t, err)
}

func liveWithDatabase(t *testing.T, s *integration.Suite, p people, releaseID string) databaseDTO {
	t.Helper()
	created := createDatabase(t, p.bob, "dev")
	require.Equal(t, http.StatusCreated, created.Status, created.Body)
	attached := attachShop(t, p.bob)
	deployPlaces(t, p.bob, releaseID)
	requireWorkingDSN(t, runToSuccess(t, s).Secrets["PLATFORM_DB_DSN"])
	return attached
}

func detachShop(t *testing.T, c *integration.Client) databaseDTO {
	t.Helper()
	var d databaseDTO
	c.MustJSON(t, http.MethodDelete, databasePath+"/attachments/places-api", nil, http.StatusOK, &d)
	return d
}

func Test_DatabaseLifecycleInDev(t *testing.T) {
	s, p, admin := setupDatabases(t)
	releaseID := builtRelease(t, s, p)
	attached := liveWithDatabase(t, s, p, releaseID)

	whileAttached := p.bob.Do(t, http.MethodDelete, databasePath, nil)
	detached := detachShop(t, p.bob)
	whileRunning := p.bob.Do(t, http.MethodDelete, databasePath, nil)
	deployPlaces(t, p.bob, releaseID)
	redeployed := runToSuccess(t, s)
	deleted := p.bob.Do(t, http.MethodDelete, databasePath, nil)

	require.Equal(t, []attachmentDTO{{Service: "places-api"}}, attached.Attachments)
	require.Equal(t, http.StatusConflict, whileAttached.Status, whileAttached.Body)
	require.Equal(t, []attachmentDTO{{Service: "places-api", Draining: true}}, detached.Attachments)
	require.Equal(t, []string{attachNote, attachNote}, []string{attached.Note, detached.Note})
	require.Equal(t, http.StatusConflict, whileRunning.Status, whileRunning.Body)
	require.Empty(t, redeployed.Secrets["PLATFORM_DB_DSN"])
	require.Equal(t, http.StatusNoContent, deleted.Status, deleted.Body)
	require.False(t, schemaExists(t, admin, devSchema))
}

func Test_DatabaseCreatedShowsItsConnectionFacts(t *testing.T) {
	_, p, _ := setupDatabases(t)
	var created, got databaseDTO
	var list struct {
		Items []databaseDTO `json:"items"`
	}

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/databases", map[string]any{"name": testDatabase, "environment": "dev", "team": "places", "max_connections": 5}, http.StatusCreated, &created)
	p.bob.MustJSON(t, http.MethodGet, databasePath, nil, http.StatusOK, &got)
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/databases?environment=dev&team=places", nil, http.StatusOK, &list)

	require.Equal(t, databaseDTO{
		Name: testDatabase, Environment: "dev", Host: "127.0.0.1", Port: 3306, Schema: devSchema,
		Username: "kz_" + devSchema, MaxConnections: 5, Attachments: []attachmentDTO{},
	}, got)
	require.Equal(t, got, created)
	require.Equal(t, []databaseDTO{got}, list.Items)
}

func Test_DatabaseInProdNeedsApproval(t *testing.T) {
	s, p, admin := setupDatabases(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/databases", map[string]any{"name": testDatabase, "environment": "prod", "team": "places"})
	before := schemaExists(t, admin, prodSchema)
	approved := decide(t, p.root, pending, "approve", url.Values{})
	var d databaseDTO
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/databases/prod/"+testDatabase, nil, http.StatusOK, &d)

	require.False(t, before)
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, "executed", poll(t, agent, pending).Status)
	require.Equal(t, prodSchema, d.Schema)
	require.True(t, schemaExists(t, admin, prodSchema))
}

type answered struct {
	status   int
	response integration.Response
}

func readsAsRoot(t *testing.T, c *integration.Client) []answered {
	t.Helper()
	reads := []answered{}
	for _, path := range []string{databasePath, "/api/v1/databases", "/api/v1/resources", "/api/v1/audit", "/api/v1/config/places-api/dev"} {
		reads = append(reads, answered{http.StatusOK, c.Do(t, http.MethodGet, path, nil)})
	}
	return reads
}

func Test_DatabaseNeverReturnsThePassword(t *testing.T) {
	s, p, _ := setupDatabases(t)
	releaseID := builtRelease(t, s, p)
	all := []answered{{http.StatusCreated, createDatabase(t, p.bob, "dev")}}
	all = append(all, answered{http.StatusOK, p.bob.Do(t, http.MethodPost, databasePath+"/attachments", map[string]string{"service": "places-api"})})
	deployPlaces(t, p.bob, releaseID)
	w := runToSuccess(t, s)
	password := w.Secrets["PLATFORM_DB_PASSWORD"]
	all = append(all, readsAsRoot(t, p.root)...)
	all = append(all, answered{http.StatusOK, p.bob.Do(t, http.MethodDelete, databasePath+"/attachments/places-api", nil)})

	require.Len(t, password, 64)
	require.Contains(t, w.Secrets["PLATFORM_DB_DSN"], password)
	for _, a := range all {
		require.Equal(t, a.status, a.response.Status, a.response.Body)
		require.NotContains(t, a.response.Body, password)
	}
}

func Test_ResourcesListDatabases(t *testing.T) {
	_, p, _ := setupDatabases(t)
	require.Equal(t, http.StatusCreated, createDatabase(t, p.bob, "dev").Status)
	attachShop(t, p.bob)
	var attached, detached resourcePage

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/resources?kind=database", nil, http.StatusOK, &attached)
	detachShop(t, p.bob)
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/resources?kind=database", nil, http.StatusOK, &detached)

	require.Equal(t, []resourceDTO{{Kind: "database", Name: testDatabase, Environment: "dev", Attached: []string{"places-api"}}}, attached.Items)
	require.Equal(t, []resourceDTO{{Kind: "database", Name: testDatabase, Environment: "dev", Attached: []string{}}}, detached.Items)
}

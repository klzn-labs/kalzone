package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/uuid"
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

const (
	oldSchema   = "kzt_old"
	oldUser     = "kz_" + oldSchema
	oldDatabase = "kzt-old"
	oldPath     = "/api/v1/databases/dev/" + oldDatabase
)

type adoptedDTO struct {
	databaseDTO
	Adopted bool `json:"adopted"`
}

func setupOldSchema(t *testing.T, admin *sql.DB) {
	t.Helper()
	t.Cleanup(func() { dropOldSchema(admin) })
	for _, statement := range []string{
		"CREATE DATABASE `" + oldSchema + "`",
		"CREATE TABLE `" + oldSchema + "`.places (name VARCHAR(32))",
		"INSERT INTO `" + oldSchema + "`.places VALUES ('lisbon')",
	} {
		_, err := admin.Exec(statement)
		require.NoError(t, err)
	}
}

func dropOldSchema(admin *sql.DB) {
	_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + oldSchema + "`")
	_, _ = admin.Exec("DROP USER IF EXISTS '" + oldUser + "'@'%'")
}

func playRootStep(t *testing.T, admin *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"CREATE USER IF NOT EXISTS '" + oldUser + "'@'%' IDENTIFIED BY '" + uuid.NewUUID() + "' ACCOUNT LOCK",
		"GRANT ALL PRIVILEGES ON `" + oldSchema + "`.* TO '" + oldUser + "'@'%'",
	} {
		_, err := admin.Exec(statement)
		require.NoError(t, err)
	}
}

func adoptBody() map[string]any {
	return map[string]any{"schema": oldSchema, "name": oldDatabase, "environment": "dev", "team": "places", "services": []string{"places-api"}}
}

func platformAdmin(t *testing.T, s *integration.Suite, p people, scopes ...string) *integration.Client {
	t.Helper()
	return agentFor(t, s, p.root, tokenBody("platform-admin", scopes...))
}

func adoptOld(t *testing.T, s *integration.Suite, p people) integration.Response {
	t.Helper()
	return platformAdmin(t, s, p, "read", "services:admin").Do(t, http.MethodPost, "/api/v1/databases/adopt", adoptBody())
}

func readPlace(t *testing.T, dsn string) string {
	t.Helper()
	conn, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	var name string
	require.NoError(t, conn.QueryRow("SELECT name FROM places").Scan(&name))
	return name
}

func userExists(t *testing.T, admin *sql.DB, user string) bool {
	t.Helper()
	var found int
	require.NoError(t, admin.QueryRow("SELECT COUNT(*) FROM mysql.user WHERE user = ?", user).Scan(&found))
	return found > 0
}

func Test_AdoptDatabase_EndToEnd(t *testing.T) {
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	playRootStep(t, admin)
	releaseID := builtRelease(t, s, p)

	adopted := adoptOld(t, s, p)
	require.Equal(t, http.StatusCreated, adopted.Status, adopted.Body)
	var d adoptedDTO
	require.NoError(t, json.Unmarshal([]byte(adopted.Body), &d))
	deployPlaces(t, p.bob, releaseID)
	w := runToSuccess(t, s)

	require.True(t, d.Adopted)
	require.Equal(t, oldUser, d.Username)
	require.Equal(t, "services get the credentials on their next deploy; the schema was not changed", d.Note)
	require.NotContains(t, adopted.Body, `"password"`)
	require.Equal(t, "lisbon", readPlace(t, w.Secrets["PLATFORM_DB_DSN"]))
}

func Test_AdoptDatabase_WithoutRootStep(t *testing.T) {
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	var refused apiError
	var list struct {
		Items []databaseDTO `json:"items"`
	}

	platformAdmin(t, s, p, "read", "services:admin").MustJSON(t, http.MethodPost, "/api/v1/databases/adopt", adoptBody(), http.StatusConflict, &refused)
	p.root.MustJSON(t, http.MethodGet, "/api/v1/databases", nil, http.StatusOK, &list)

	require.Contains(t, refused.Error.Hint, "adopt-schema.sh "+oldSchema)
	require.Empty(t, list.Items)
}

func Test_AdoptDatabase_NeedsServicesAdmin(t *testing.T) {
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	playRootStep(t, admin)
	var refused apiError

	platformAdmin(t, s, p, "read", "services").MustJSON(t, http.MethodPost, "/api/v1/databases/adopt", adoptBody(), http.StatusForbidden, &refused)

	require.Equal(t, "err_missing_scope", refused.Error.Code)
}

func Test_ReleaseDatabase_KeepsTheData(t *testing.T) {
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	playRootStep(t, admin)
	releaseID := builtRelease(t, s, p)
	require.Equal(t, http.StatusCreated, adoptOld(t, s, p).Status)
	p.bob.MustJSON(t, http.MethodDelete, oldPath+"/attachments/places-api", nil, http.StatusOK, nil)
	deployPlaces(t, p.bob, releaseID)
	runToSuccess(t, s)
	var released struct {
		Released bool   `json:"released"`
		Message  string `json:"message"`
	}

	p.bob.MustJSON(t, http.MethodDelete, oldPath, nil, http.StatusOK, &released)

	require.True(t, released.Released)
	require.Equal(t, "the platform's user and record are removed; the schema and its data stay in MySQL", released.Message)
	require.Equal(t, "lisbon", placeAsRoot(t, admin))
	require.False(t, userExists(t, admin, oldUser))
}

func Test_ReleaseDatabase_InProdWaitsForApproval(t *testing.T) {
	s, p, admin := setupDatabases(t)
	setupOldSchema(t, admin)
	playRootStep(t, admin)
	body := adoptBody()
	body["environment"], body["services"] = "prod", []string{}
	platformAdmin(t, s, p, "read", "services:admin").MustJSON(t, http.MethodPost, "/api/v1/databases/adopt", body, http.StatusCreated, nil)

	answer := prodAgent(t, s, p).Do(t, http.MethodDelete, "/api/v1/databases/prod/"+oldDatabase, nil)

	require.Equal(t, http.StatusAccepted, answer.Status, answer.Body)
	require.NotContains(t, answer.Body, `"released"`)
	p.root.MustJSON(t, http.MethodGet, "/api/v1/databases/prod/"+oldDatabase, nil, http.StatusOK, nil)
	require.True(t, userExists(t, admin, oldUser))
	require.Equal(t, "lisbon", placeAsRoot(t, admin))
}

func placeAsRoot(t *testing.T, admin *sql.DB) string {
	t.Helper()
	var name string
	require.NoError(t, admin.QueryRow("SELECT name FROM `"+oldSchema+"`.places").Scan(&name))
	return name
}

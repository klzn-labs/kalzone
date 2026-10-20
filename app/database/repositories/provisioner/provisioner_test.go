package provisioner_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/database/repositories/provisioner"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

const testPassword = `p@ss'w"ord;--`

func adminConfig(t *testing.T) *driver.Config {
	t.Helper()
	c, err := driver.ParseDSN(testdb.DSN())
	require.NoError(t, err)
	c.DBName = ""
	return c
}

func openAdmin(t *testing.T) *sql.DB {
	t.Helper()
	admin, err := sql.Open("mysql", adminConfig(t).FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	return admin
}

func newProvisioner(t *testing.T) *provisioner.Provisioner {
	t.Helper()
	cfg := config.Config{Databases: config.Databases{AdminDSN: adminConfig(t).FormatDSN()}}
	p, err := provisioner.New(cfg)
	require.NoError(t, err)
	return p
}

func scratch(t *testing.T, name string) (string, string) {
	t.Helper()
	schema, user := "kzt_"+name, "kz_kzt_"+name
	admin := openAdmin(t)
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`")
		_, _ = admin.Exec("DROP USER IF EXISTS '" + user + "'@'%'")
	})
	return schema, user
}

func connectAs(t *testing.T, user, password string) *sql.DB {
	t.Helper()
	c := adminConfig(t)
	c.User, c.Passwd = user, password
	conn, err := sql.Open("mysql", c.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func visibleDatabases(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	rows, err := conn.Query("SHOW DATABASES")
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	return names
}

func Test_Create_UserSeesOnlyItsSchema(t *testing.T) {
	schema, user := scratch(t, "scope")
	p := newProvisioner(t)

	require.NoError(t, p.Create(context.Background(), schema, user, testPassword, 5))

	conn := connectAs(t, user, testPassword)
	require.ElementsMatch(t, []string{"information_schema", "performance_schema", schema}, visibleDatabases(t, conn))
	_, err := conn.Exec("CREATE TABLE `" + schema + "`.t (id INT)")
	require.NoError(t, err)
	_, err = conn.Exec("SELECT * FROM platform_test.users")
	var myErr *driver.MySQLError
	require.ErrorAs(t, err, &myErr)
	require.Equal(t, uint16(1142), myErr.Number)
}

func Test_Create_CapsConnections(t *testing.T) {
	schema, user := scratch(t, "cap")
	p := newProvisioner(t)

	require.NoError(t, p.Create(context.Background(), schema, user, "pw", 7))

	var got int
	row := openAdmin(t).QueryRow("SELECT max_user_connections FROM mysql.user WHERE user = ?", user)
	require.NoError(t, row.Scan(&got))
	require.Equal(t, 7, got)
}

func Test_Create_RollsBackOnUserClash(t *testing.T) {
	schema, user := scratch(t, "clash")
	admin := openAdmin(t)
	_, err := admin.Exec(fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY 'x'", user))
	require.NoError(t, err)
	p := newProvisioner(t)

	require.Error(t, p.Create(context.Background(), schema, user, "pw", 5))

	exists, err := p.SchemaExists(context.Background(), schema)
	require.NoError(t, err)
	require.False(t, exists)
	var count int
	require.NoError(t, admin.QueryRow("SELECT COUNT(*) FROM mysql.user WHERE user = ?", user).Scan(&count))
	require.Equal(t, 1, count)
}

func Test_Create_ReportsAnExistingSchemaAsConflict(t *testing.T) {
	schema, user := scratch(t, "taken")
	_, err := openAdmin(t).Exec("CREATE DATABASE `" + schema + "`")
	require.NoError(t, err)
	_, err = openAdmin(t).Exec("CREATE TABLE `" + schema + "`.t (id INT)")
	require.NoError(t, err)

	err = newProvisioner(t).Create(context.Background(), schema, user, "pw", 5)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Contains(t, err.Error(), "already exists in MySQL")
	var tables int
	require.NoError(t, openAdmin(t).QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", schema).Scan(&tables))
	require.Equal(t, 1, tables)
}

func Test_Create_ReportsASchemaItCannotTouchAsConflict(t *testing.T) {
	schema, user := scratch(t, "fenced")
	fenced := limitedProvisioner(t)

	err := fenced.Create(context.Background(), schema, user, "pw", 5)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Contains(t, err.Error(), "already exists in MySQL")
}

func limitedProvisioner(t *testing.T) *provisioner.Provisioner {
	t.Helper()
	admin := openAdmin(t)
	_, err := admin.Exec("CREATE USER IF NOT EXISTS 'kzt_limited'@'%' IDENTIFIED BY 'pw'")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec("DROP USER IF EXISTS 'kzt_limited'@'%'") })
	c := adminConfig(t)
	c.User, c.Passwd = "kzt_limited", "pw"
	p, err := provisioner.New(config.Config{Databases: config.Databases{AdminDSN: c.FormatDSN()}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func Test_Drop_IsIdempotent(t *testing.T) {
	schema, user := scratch(t, "drop")
	p := newProvisioner(t)
	require.NoError(t, p.Create(context.Background(), schema, user, "pw", 5))

	require.NoError(t, p.Drop(context.Background(), schema, user))
	require.NoError(t, p.Drop(context.Background(), schema, user))

	exists, err := p.SchemaExists(context.Background(), schema)
	require.NoError(t, err)
	require.False(t, exists)
}

func Test_RefusesUnsafeIdentifiers(t *testing.T) {
	p := newProvisioner(t)
	for _, bad := range []string{"a;drop", "a`", "", "A", "a b"} {
		err := p.Create(context.Background(), bad, "kz_ok", "pw", 5)
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err), bad)
		err = p.Create(context.Background(), "kzt_ok", bad, "pw", 5)
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err), bad)
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(p.Drop(context.Background(), bad, "kz_ok")), bad)
	}
}

func Test_WithoutAnAdminDSN_EveryCallIsNotConfigured(t *testing.T) {
	p, err := provisioner.New(config.Config{})
	require.NoError(t, err)
	ctx := context.Background()

	_, existsErr := p.SchemaExists(ctx, "kzt_x")

	calls := []error{existsErr, p.Create(ctx, "kzt_x", "kz_x", "pw", 5), p.Drop(ctx, "kzt_x", "kz_x"), p.Adopt(ctx, "kzt_x", "kz_kzt_x", "pw", 5), p.Release(ctx, "kz_kzt_x")}
	for _, got := range calls {
		require.Equal(t, apperror.CodeNotConfigured, apperror.CodeOf(got))
	}
}

func Test_RefusesReservedSchemas(t *testing.T) {
	p := newProvisioner(t)
	for _, reserved := range []string{"mysql", "sys", "information_schema", "performance_schema", "platform"} {
		err := p.Create(context.Background(), reserved, "kz_ok", "pw", 5)
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err), reserved)
		err = p.Drop(context.Background(), reserved, "kz_ok")
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err), reserved)
	}
}

func Test_RefusesMismatchedUser(t *testing.T) {
	p := newProvisioner(t)
	ctx := context.Background()
	for _, user := range []string{"root", "mysql", "kz_kzt_b", "kz_", "kzt_a"} {
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(p.Create(ctx, "kzt_a", user, "pw", 5)), user)
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(p.Drop(ctx, "kzt_a", user)), user)
	}
}

func Test_New_RefusesAnAdminDSNWithACharset(t *testing.T) {
	dsn := "root:test@tcp(127.0.0.1:3306)/?charset=latin1"
	_, err := provisioner.New(config.Config{Databases: config.Databases{AdminDSN: dsn}})
	require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err))
}

func prepareForAdoption(t *testing.T, schema, user string) {
	t.Helper()
	admin := openAdmin(t)
	_, err := admin.Exec("CREATE USER IF NOT EXISTS '" + user + "'@'%' IDENTIFIED BY '" + strings.Repeat("ab", 32) + "' ACCOUNT LOCK")
	require.NoError(t, err)
	_, err = admin.Exec("GRANT ALL PRIVILEGES ON `" + schema + "`.* TO '" + user + "'@'%'")
	require.NoError(t, err)
}

func existingSchema(t *testing.T, name string) (string, string) {
	t.Helper()
	schema, user := scratch(t, name)
	admin := openAdmin(t)
	for _, statement := range []string{
		"CREATE DATABASE `" + schema + "`",
		"CREATE TABLE `" + schema + "`.t (id INT PRIMARY KEY, name VARCHAR(20))",
		"INSERT INTO `" + schema + "`.t VALUES (1, 'kept')",
	} {
		_, err := admin.Exec(statement)
		require.NoError(t, err)
	}
	return schema, user
}

func accountLocked(t *testing.T, user string) bool {
	t.Helper()
	var locked string
	require.NoError(t, openAdmin(t).QueryRow("SELECT account_locked FROM mysql.user WHERE user = ?", user).Scan(&locked))
	return locked == "Y"
}

func userCount(t *testing.T, user string) int {
	t.Helper()
	var count int
	require.NoError(t, openAdmin(t).QueryRow("SELECT COUNT(*) FROM mysql.user WHERE user = ?", user).Scan(&count))
	return count
}

func schemaSnapshot(t *testing.T, schema string) string {
	t.Helper()
	admin := openAdmin(t)
	var table, definition, name string
	require.NoError(t, admin.QueryRow("SHOW CREATE TABLE `"+schema+"`.t").Scan(&table, &definition))
	require.NoError(t, admin.QueryRow("SELECT GROUP_CONCAT(CONCAT(id, ':', name) ORDER BY id) FROM `"+schema+"`.t").Scan(&name))
	return definition + "|" + name
}

const secretPassword = "pw-SECRET-123"

func requireNotPrepared(t *testing.T, err error, schema string) {
	t.Helper()
	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Contains(t, err.Error(), "not been prepared for adoption")
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	require.Contains(t, appErr.Hint, "adopt-schema.sh "+schema)
	require.NotContains(t, err.Error(), secretPassword)
	require.NotContains(t, appErr.Hint, secretPassword)
}

func userCreatorProvisioner(t *testing.T) *provisioner.Provisioner {
	t.Helper()
	admin := openAdmin(t)
	t.Cleanup(func() { _, _ = admin.Exec("DROP USER IF EXISTS 'kzt_creator'@'%'") })
	_, err := admin.Exec("CREATE USER IF NOT EXISTS 'kzt_creator'@'%' IDENTIFIED BY 'pw'")
	require.NoError(t, err)
	_, err = admin.Exec("GRANT CREATE USER ON *.* TO 'kzt_creator'@'%'")
	require.NoError(t, err)
	c := adminConfig(t)
	c.User, c.Passwd = "kzt_creator", "pw"
	p, err := provisioner.New(config.Config{Databases: config.Databases{AdminDSN: c.FormatDSN()}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func Test_Adopt_NeedsOnlyCreateUserOnTheAdminSide(t *testing.T) {
	schema, user := existingSchema(t, "ad")
	prepareForAdoption(t, schema, user)
	fenced := userCreatorProvisioner(t)
	ctx := context.Background()

	require.NoError(t, fenced.Adopt(ctx, schema, user, "pw", 7))

	var name string
	require.NoError(t, connectAs(t, user, "pw").QueryRow("SELECT name FROM `"+schema+"`.t WHERE id = 1").Scan(&name))
	require.Equal(t, "kept", name)
	require.NoError(t, fenced.Release(ctx, user))
	require.Equal(t, 0, userCount(t, user))
}

func Test_RefusesAUserLongerThan32Characters(t *testing.T) {
	p := newProvisioner(t)
	ctx := context.Background()
	schema := "kzt_" + strings.Repeat("a", 26)
	user := "kz_" + schema
	require.Len(t, user, 33)

	for _, got := range []error{p.Adopt(ctx, schema, user, "pw", 7), p.Create(ctx, schema, user, "pw", 7), p.Release(ctx, user)} {
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(got))
	}
}

func Test_Adopt_SetsThePasswordAndUnlocks(t *testing.T) {
	schema, user := existingSchema(t, "ad")
	prepareForAdoption(t, schema, user)

	require.NoError(t, newProvisioner(t).Adopt(context.Background(), schema, user, "pw", 7))

	var name string
	require.NoError(t, connectAs(t, user, "pw").QueryRow("SELECT name FROM `"+schema+"`.t WHERE id = 1").Scan(&name))
	require.Equal(t, "kept", name)
	var maxConnections int
	require.NoError(t, openAdmin(t).QueryRow("SELECT max_user_connections FROM mysql.user WHERE user = ?", user).Scan(&maxConnections))
	require.Equal(t, 7, maxConnections)
	require.False(t, accountLocked(t, user))
}

func Test_Adopt_RefusesWithoutTheRootStep(t *testing.T) {
	schema, user := existingSchema(t, "ad")

	err := newProvisioner(t).Adopt(context.Background(), schema, user, secretPassword, 7)

	requireNotPrepared(t, err, schema)
	require.Equal(t, 0, userCount(t, user))
}

func Test_Adopt_RefusesAUserWithoutTheGrant(t *testing.T) {
	schema, user := existingSchema(t, "ad")
	_, err := openAdmin(t).Exec("CREATE USER '" + user + "'@'%' IDENTIFIED BY 'x' ACCOUNT LOCK")
	require.NoError(t, err)

	err = newProvisioner(t).Adopt(context.Background(), schema, user, secretPassword, 7)

	requireNotPrepared(t, err, schema)
	require.True(t, accountLocked(t, user))
}

func Test_Adopt_LeavesTheSchemaAlone(t *testing.T) {
	schema, user := existingSchema(t, "ad")
	prepareForAdoption(t, schema, user)
	before := schemaSnapshot(t, schema)

	require.NoError(t, newProvisioner(t).Adopt(context.Background(), schema, user, "pw", 7))

	require.Equal(t, before, schemaSnapshot(t, schema))
}

func Test_Adopt_RefusesInvalidInput(t *testing.T) {
	p := newProvisioner(t)
	ctx := context.Background()
	for _, got := range []error{
		p.Adopt(ctx, "kzt_ad", "kz_kzt_ad", "", 7),
		p.Adopt(ctx, "kzt_ad", "kz_kzt_ad", "pw", 0),
		p.Adopt(ctx, "kzt_ad", "kz_kzt_ad", "pw", 31),
		p.Adopt(ctx, "kzt_ad", "plateful", "pw", 7),
		p.Adopt(ctx, "mysql", "kz_mysql", "pw", 7),
	} {
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(got))
	}
}

func Test_Release_DropsOnlyTheUser(t *testing.T) {
	schema, user := existingSchema(t, "ad")
	prepareForAdoption(t, schema, user)
	p := newProvisioner(t)
	ctx := context.Background()
	require.NoError(t, p.Adopt(ctx, schema, user, "pw", 7))
	before := schemaSnapshot(t, schema)

	require.NoError(t, p.Release(ctx, user))
	require.NoError(t, p.Release(ctx, user))

	require.Equal(t, 0, userCount(t, user))
	require.Equal(t, before, schemaSnapshot(t, schema))
}

func Test_Release_RefusesANonPlatformUser(t *testing.T) {
	p := newProvisioner(t)
	for _, user := range []string{"plateful", "root", "kz_", "kz_mysql", "kz_a;drop", "KZ_A", ""} {
		require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(p.Release(context.Background(), user)), user)
	}
}

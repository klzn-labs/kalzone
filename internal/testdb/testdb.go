package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/adrianozp/platform/internal/db"
	"github.com/adrianozp/platform/internal/migrator"
	"github.com/adrianozp/platform/pkg/config"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const defaultDSN = "root:test@tcp(127.0.0.1:3306)/platform_test"

var tables = []string{"users", "sessions", "teams", "memberships", "services", "config_revisions", "secret_versions", "audit_events"}

var prepare sync.Once

func DSN() string {
	if dsn := os.Getenv("PLATFORM_TEST_DSN"); dsn != "" {
		return dsn
	}
	return defaultDSN
}

func Config() config.Config {
	cfg := config.Config{DB: config.DB{DSN: DSN(), MaxOpenConns: 5}}
	return cfg
}

func Open(t *testing.T) *gorm.DB {
	t.Helper()
	prepare.Do(func() { require.NoError(t, createAndMigrate()) })
	gdb, err := db.NewGormDB(Config())
	require.NoError(t, err)
	Truncate(t, gdb)
	return gdb
}

func Truncate(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	for _, table := range tables {
		require.NoError(t, gdb.Exec("DELETE FROM "+table).Error)
	}
}

func createAndMigrate() error {
	if err := createDatabase(); err != nil {
		return err
	}
	m, err := migrator.New(Config())
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil {
		return err
	}
	return m.Close()
}

func createDatabase() error {
	c, err := driver.ParseDSN(DSN())
	if err != nil {
		return err
	}
	name := c.DBName
	c.DBName = ""
	conn, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", name))
	return err
}

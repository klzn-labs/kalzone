package migrator

import (
	"database/sql"
	"errors"
	"fmt"

	driver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/klzn-labs/kalzone/migrations"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var ErrDirty = errors.New("migrator: database is dirty, fix the schema by hand and force the version")

type Migrator struct {
	migrate *migrate.Migrate
}

func New(cfg config.Config) (*Migrator, error) {
	dsn, err := multiStatementDSN(cfg.DB.DSN)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	target, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return build(target)
}

func build(target database.Driver) (*Migrator, error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, err
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", target)
	if err != nil {
		return nil, err
	}
	return &Migrator{migrate: m}, nil
}

func multiStatementDSN(dsn string) (string, error) {
	c, err := driver.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	c.MultiStatements = true
	return c.FormatDSN(), nil
}

func (m *Migrator) Up() error {
	version, dirty, err := m.migrate.Version()
	if dirty {
		return fmt.Errorf("%w (version %d)", ErrDirty, version)
	}
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return err
	}
	if err := m.migrate.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func (m *Migrator) Close() error {
	sourceErr, databaseErr := m.migrate.Close()
	return errors.Join(sourceErr, databaseErr)
}

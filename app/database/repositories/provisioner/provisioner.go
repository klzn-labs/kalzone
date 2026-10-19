package provisioner

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const (
	adminConnections = 3
	adminCollation   = "utf8mb4_0900_ai_ci"
	rollbackTimeout  = 30 * time.Second
)

var identifier = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var errNotConfigured = apperror.New(apperror.CodeNotConfigured, "managed databases are not configured on this platform")

var errSchemaExists = apperror.Conflict("a schema with this name already exists in MySQL").WithHint("choose another database name")

const (
	errDatabaseExists       = 1007
	errDatabaseAccessDenied = 1044
)

type Provisioner struct {
	db *sql.DB
}

func New(cfg config.Config) (*Provisioner, error) {
	if cfg.Databases.AdminDSN == "" {
		return &Provisioner{}, nil
	}
	parsed, err := driver.ParseDSN(cfg.Databases.AdminDSN)
	if err != nil {
		return nil, err
	}
	if setsCharset(parsed) {
		return nil, apperror.Invalid("the admin DSN must not set a charset")
	}
	parsed.Collation = adminCollation
	parsed.InterpolateParams = true
	db, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(adminConnections)
	return &Provisioner{db: db}, nil
}

func setsCharset(parsed *driver.Config) bool {
	_, param := parsed.Params["charset"]
	return param || strings.Contains(parsed.FormatDSN(), "charset=")
}

func (p *Provisioner) SchemaExists(ctx context.Context, schema string) (bool, error) {
	if err := p.checkSchema(schema); err != nil {
		return false, err
	}
	var found int
	row := p.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", schema)
	if err := row.Scan(&found); err != nil {
		return false, err
	}
	return found > 0, nil
}

func (p *Provisioner) Create(ctx context.Context, schema, user, password string, maxConnections int) error {
	if err := p.checkCreate(schema, user, password, maxConnections); err != nil {
		return err
	}
	if err := p.createSchema(ctx, schema); err != nil {
		return err
	}
	if err := p.createUser(ctx, user, password, maxConnections); err != nil {
		return p.rollback(ctx, schema, "", err)
	}
	if err := p.grant(ctx, schema, user); err != nil {
		return p.rollback(ctx, schema, user, err)
	}
	return nil
}

func (p *Provisioner) Drop(ctx context.Context, schema, user string) error {
	if err := p.check(schema, user); err != nil {
		return err
	}
	if _, err := p.db.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+schema+"`"); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, "DROP USER IF EXISTS '"+user+"'@'%'")
	return err
}

func (p *Provisioner) checkCreate(schema, user, password string, maxConnections int) error {
	if err := p.check(schema, user); err != nil {
		return err
	}
	if password == "" {
		return apperror.Invalid("database password must not be empty")
	}
	return entities.ValidateMaxConnections(maxConnections)
}

func (p *Provisioner) checkSchema(schema string) error {
	if p.db == nil {
		return errNotConfigured
	}
	if !identifier.MatchString(schema) {
		return apperror.Invalid("identifier must match ^[a-z0-9_]{1,64}$")
	}
	return nil
}

func (p *Provisioner) check(schema, user string) error {
	if p.db == nil {
		return errNotConfigured
	}
	if entities.Reserved(schema) {
		return apperror.Invalid("schema name is reserved")
	}
	if !identifier.MatchString(schema) || !identifier.MatchString(user) {
		return apperror.Invalid("identifier must match ^[a-z0-9_]{1,64}$")
	}
	if user != entities.UsernamePrefix+schema {
		return apperror.Invalid("database user must be " + entities.UsernamePrefix + "<schema>")
	}
	return nil
}

func (p *Provisioner) createSchema(ctx context.Context, schema string) error {
	_, err := p.db.ExecContext(ctx, "CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
	if schemaTaken(err) {
		return errSchemaExists
	}
	return err
}

func schemaTaken(err error) bool {
	var myErr *driver.MySQLError
	if !errors.As(err, &myErr) {
		return false
	}
	return myErr.Number == errDatabaseExists || myErr.Number == errDatabaseAccessDenied
}

func (p *Provisioner) createUser(ctx context.Context, user, password string, maxConnections int) error {
	statement := "CREATE USER '" + user + "'@'%' IDENTIFIED BY ? WITH MAX_USER_CONNECTIONS " + strconv.Itoa(maxConnections)
	_, err := p.db.ExecContext(ctx, statement, password)
	return err
}

func (p *Provisioner) grant(ctx context.Context, schema, user string) error {
	_, err := p.db.ExecContext(ctx, "GRANT ALL PRIVILEGES ON `"+schema+"`.* TO '"+user+"'@'%'")
	return err
}

func (p *Provisioner) rollback(ctx context.Context, schema, user string, cause error) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	_, _ = p.db.ExecContext(cleanup, "DROP DATABASE IF EXISTS `"+schema+"`")
	if user != "" {
		_, _ = p.db.ExecContext(cleanup, "DROP USER IF EXISTS '"+user+"'@'%'")
	}
	return cause
}

func (p *Provisioner) Close() error {
	if p.db == nil {
		return nil
	}
	return p.db.Close()
}

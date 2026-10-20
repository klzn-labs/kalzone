package databases

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

const (
	defaultMaxConnections = 10
	undoTimeout           = 30 * time.Second
)

type Draft struct {
	Name           string `json:"name"`
	Environment    string `json:"environment"`
	Team           string `json:"team"`
	MaxConnections int    `json:"max_connections"`
}

var (
	errReserved     = apperror.Invalid("this database name maps to a schema the platform reserves").WithHint("choose another database name")
	errNameTaken    = apperror.Conflict("a database with this name already exists in this environment")
	errSchemaTaken  = apperror.Conflict("another database already uses this schema").WithHint("choose another database name")
	errSchemaExists = apperror.Conflict("a schema with this name already exists in MySQL").WithHint("choose another database name")
)

func (u DatabasesUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (entities.Database, error) {
	d = withDefaults(d)
	db, err := u.planned(ctx, d)
	if err != nil {
		return entities.Database{}, err
	}
	gate, err := u.authz.Gate(ctx, a, authz.EditDatabase, resourceOf(db))
	if err != nil {
		return entities.Database{}, err
	}
	if err := u.checkFree(ctx, db); err != nil {
		return entities.Database{}, err
	}
	if gate == authz.NeedsApproval {
		return entities.Database{}, u.propose(ctx, a, db, Proposal{Kind: approval.KindDatabaseCreate, Draft: &d})
	}
	return u.create(ctx, a, gate, db)
}

func withDefaults(d Draft) Draft {
	if d.MaxConnections == 0 {
		d.MaxConnections = defaultMaxConnections
	}
	return d
}

func (u DatabasesUsecase) planned(ctx context.Context, d Draft) (entities.Database, error) {
	if err := validate(d); err != nil {
		return entities.Database{}, err
	}
	teamID, err := u.catalog.FindTeam(ctx, d.Team)
	if err != nil {
		return entities.Database{}, err
	}
	schema, err := u.schemaOf(ctx, d)
	if err != nil {
		return entities.Database{}, err
	}
	user, err := entities.UsernameFor(schema)
	now := clock.Now().UTC()
	return entities.Database{
		ID: uuid.NewUUID(), Name: d.Name, Environment: d.Environment, TeamID: teamID, Schema: schema,
		Username: user, MaxConnections: d.MaxConnections, CreatedAt: now, UpdatedAt: now,
	}, err
}

func validate(d Draft) error {
	if err := entities.ValidateName(d.Name); err != nil {
		return err
	}
	return entities.ValidateMaxConnections(d.MaxConnections)
}

func (u DatabasesUsecase) schemaOf(ctx context.Context, d Draft) (string, error) {
	protected, err := u.catalog.IsProtected(ctx, d.Environment)
	if err != nil {
		return "", err
	}
	return entities.SchemaFor(d.Name, d.Environment, protected), nil
}

func (u DatabasesUsecase) checkFree(ctx context.Context, db entities.Database) error {
	if entities.Reserved(db.Schema) {
		return errReserved
	}
	all, err := u.store.List(ctx, "", "")
	if err != nil {
		return err
	}
	if slices.ContainsFunc(all, func(o entities.Database) bool { return o.Environment == db.Environment && o.Name == db.Name }) {
		return errNameTaken
	}
	if slices.ContainsFunc(all, func(o entities.Database) bool { return o.Schema == db.Schema }) {
		return errSchemaTaken
	}
	return u.checkSchemaAbsent(ctx, db.Schema)
}

func (u DatabasesUsecase) create(ctx context.Context, a actor.Actor, gate authz.Gate, db entities.Database) (entities.Database, error) {
	password, err := u.sealedPassword(&db)
	if err != nil {
		return entities.Database{}, err
	}
	if err := u.provisioner.Create(ctx, db.Schema, db.Username, password, db.MaxConnections); err != nil {
		return entities.Database{}, err
	}
	if err := u.store.Create(ctx, db); err != nil {
		return entities.Database{}, u.undo(ctx, db, err, func(c context.Context) error { return u.provisioner.Drop(c, db.Schema, db.Username) })
	}
	u.publish(ctx, a, gate, "database.created", db, map[string]any{"schema": db.Schema, "max_connections": db.MaxConnections})
	return db, nil
}

func (u DatabasesUsecase) checkSchemaAbsent(ctx context.Context, schema string) error {
	exists, err := u.provisioner.SchemaExists(ctx, schema)
	if err != nil {
		return err
	}
	if exists {
		return errSchemaExists
	}
	return nil
}

func (u DatabasesUsecase) undo(ctx context.Context, db entities.Database, cause error, revert func(context.Context) error) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	_, err := u.store.GetByID(cleanup, db.ID)
	if apperror.CodeOf(err) != apperror.CodeNotFound {
		return errors.Join(cause, err)
	}
	if err := revert(cleanup); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

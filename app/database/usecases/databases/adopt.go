package databases

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type AdoptDraft struct {
	Schema         string   `json:"schema"`
	Name           string   `json:"name"`
	Environment    string   `json:"environment"`
	Team           string   `json:"team"`
	MaxConnections int      `json:"max_connections"`
	Services       []string `json:"services"`
}

var schemaPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

var (
	errInvalidSchema  = apperror.Invalid("schema must be lowercase letters, digits or underscores")
	errServiceTwice   = apperror.Invalid("a service is listed more than once")
	errAdoptReserved  = apperror.Invalid("the platform reserves this schema")
	errSchemaInUse    = apperror.Conflict("another database already uses this schema").WithHint("a schema is adopted once; use the database that holds it")
	errSchemaNotFound = apperror.NotFound("no schema with this name exists in MySQL").WithHint("check the schema name in MySQL")
)

func (u DatabasesUsecase) Adopt(ctx context.Context, a actor.Actor, d AdoptDraft) (entities.Database, error) {
	db, err := u.plannedAdoption(ctx, d)
	if err != nil {
		return entities.Database{}, err
	}
	gate, err := authz.Direct(u.authz.Gate(ctx, a, authz.AdoptDatabase, resourceOf(db)))
	if err != nil {
		return entities.Database{}, err
	}
	if err := u.checkAdoptable(ctx, db); err != nil {
		return entities.Database{}, err
	}
	svcs, err := u.adoptableServices(ctx, a, db, d.Services)
	if err != nil {
		return entities.Database{}, err
	}
	return u.adopt(ctx, a, gate, db, svcs)
}

func (u DatabasesUsecase) plannedAdoption(ctx context.Context, d AdoptDraft) (entities.Database, error) {
	if d.MaxConnections == 0 {
		d.MaxConnections = defaultMaxConnections
	}
	if err := validateAdoption(d); err != nil {
		return entities.Database{}, err
	}
	user, err := entities.UsernameFor(d.Schema)
	if err != nil {
		return entities.Database{}, err
	}
	teamID, err := u.catalog.FindTeam(ctx, d.Team)
	now := clock.Now().UTC()
	return entities.Database{
		ID: uuid.NewUUID(), Name: d.Name, Environment: d.Environment, TeamID: teamID, Schema: d.Schema,
		Username: user, MaxConnections: d.MaxConnections, Adopted: true, CreatedAt: now, UpdatedAt: now,
	}, err
}

func validateAdoption(d AdoptDraft) error {
	if !schemaPattern.MatchString(d.Schema) {
		return errInvalidSchema
	}
	if err := entities.ValidateName(d.Name); err != nil {
		return err
	}
	if err := entities.ValidateMaxConnections(d.MaxConnections); err != nil {
		return err
	}
	if len(slices.Compact(slices.Sorted(slices.Values(d.Services)))) != len(d.Services) {
		return errServiceTwice
	}
	return nil
}

func (u DatabasesUsecase) checkAdoptable(ctx context.Context, db entities.Database) error {
	if entities.Reserved(db.Schema) {
		return errAdoptReserved
	}
	all, err := u.store.List(ctx, "", "")
	if err != nil {
		return err
	}
	if slices.ContainsFunc(all, func(o entities.Database) bool { return o.Schema == db.Schema }) {
		return errSchemaInUse
	}
	if err := u.checkSchemaPresent(ctx, db.Schema); err != nil {
		return err
	}
	if slices.ContainsFunc(all, func(o entities.Database) bool { return o.Environment == db.Environment && o.Name == db.Name }) {
		return errNameTaken
	}
	return nil
}

func (u DatabasesUsecase) checkSchemaPresent(ctx context.Context, schema string) error {
	exists, err := u.provisioner.SchemaExists(ctx, schema)
	if err != nil {
		return err
	}
	if !exists {
		return errSchemaNotFound
	}
	return nil
}

func (u DatabasesUsecase) adoptableServices(ctx context.Context, a actor.Actor, db entities.Database, slugs []string) ([]ServiceRef, error) {
	if _, err := u.catalog.IsProtected(ctx, db.Environment); err != nil {
		return nil, err
	}
	svcs := make([]ServiceRef, 0, len(slugs))
	for _, slug := range slugs {
		svc, err := u.adoptableService(ctx, a, db, slug)
		if err != nil {
			return nil, err
		}
		svcs = append(svcs, svc)
	}
	return svcs, nil
}

func (u DatabasesUsecase) adoptableService(ctx context.Context, a actor.Actor, db entities.Database, slug string) (ServiceRef, error) {
	svc, err := u.catalog.FindService(ctx, slug)
	if err != nil {
		return ServiceRef{}, err
	}
	owner := authz.Resource{TeamID: svc.OwnerTeamID, Environment: db.Environment}
	if _, err := authz.Direct(u.authz.Gate(ctx, a, authz.AdoptDatabase, owner)); err != nil {
		return ServiceRef{}, err
	}
	current, attached, err := u.store.ActiveFor(ctx, svc.ID, db.Environment)
	if err != nil {
		return ServiceRef{}, err
	}
	if attached {
		return ServiceRef{}, errOtherAttached.WithHint("detach " + current.Name + " from " + svc.Slug + " first")
	}
	return svc, nil
}

func (u DatabasesUsecase) adopt(ctx context.Context, a actor.Actor, gate authz.Gate, db entities.Database, svcs []ServiceRef) (entities.Database, error) {
	password, err := u.sealedPassword(&db)
	if err != nil {
		return entities.Database{}, err
	}
	if err := u.store.Create(ctx, db); err != nil {
		return entities.Database{}, u.discard(ctx, db, err)
	}
	if err := u.provisioner.Adopt(ctx, db.Schema, db.Username, password, db.MaxConnections); err != nil {
		return entities.Database{}, u.discard(ctx, db, err)
	}
	slugs, err := u.attachAll(ctx, db, svcs)
	u.publish(ctx, a, gate, "database.adopted", db, map[string]any{"schema": db.Schema, "max_connections": db.MaxConnections, "services": slugs})
	if err != nil {
		return entities.Database{}, err
	}
	return db, nil
}

func (u DatabasesUsecase) discard(ctx context.Context, db entities.Database, cause error) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	if err := u.store.Delete(cleanup, db.ID); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (u DatabasesUsecase) attachAll(ctx context.Context, db entities.Database, svcs []ServiceRef) (string, error) {
	now := clock.Now().UTC()
	slugs := make([]string, 0, len(svcs))
	for _, svc := range svcs {
		attachment := entities.Attachment{DatabaseID: db.ID, ServiceID: svc.ID, Environment: db.Environment, AttachedAt: now}
		if err := u.store.Attach(ctx, attachment); err != nil {
			return strings.Join(slugs, ", "), err
		}
		slugs = append(slugs, svc.Slug)
	}
	return strings.Join(slugs, ", "), nil
}

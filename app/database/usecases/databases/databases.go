package databases

import (
	"context"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type ServiceRef struct {
	ID          string
	Slug        string
	OwnerTeamID string
}

type Store interface {
	Create(ctx context.Context, d entities.Database) error
	Get(ctx context.Context, environment, name string) (entities.Database, error)
	GetByID(ctx context.Context, id string) (entities.Database, error)
	List(ctx context.Context, environment, teamID string) ([]entities.Database, error)
	Delete(ctx context.Context, id string) error
	Attach(ctx context.Context, a entities.Attachment) error
	Detach(ctx context.Context, databaseID, serviceID string, at time.Time) error
	Attachments(ctx context.Context, databaseID string) ([]entities.Attachment, error)
	ActiveFor(ctx context.Context, serviceID, environment string) (entities.Database, bool, error)
	ForgetDrained(ctx context.Context, databaseID, serviceID string) error
	DetachService(ctx context.Context, serviceID string, at time.Time) error
}

type Provisioner interface {
	SchemaExists(ctx context.Context, schema string) (bool, error)
	Create(ctx context.Context, schema, user, password string, maxConnections int) error
	Drop(ctx context.Context, schema, user string) error
	Adopt(ctx context.Context, schema, user, password string, maxConnections int) error
	Release(ctx context.Context, user string) error
}

type Catalog interface {
	FindService(ctx context.Context, slug string) (ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (ServiceRef, error)
	FindTeam(ctx context.Context, slug string) (string, error)
	TeamSlug(ctx context.Context, id string) (string, error)
	IsProtected(ctx context.Context, environment string) (bool, error)
}

type Running interface {
	RunsSince(ctx context.Context, serviceID, environment string, since time.Time) (bool, error)
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
	Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error)
}

type Authorizer interface {
	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
}

type Requests interface {
	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type Endpoint struct {
	Host string
	Port int
}

type DatabasesUsecase struct {
	store       Store
	provisioner Provisioner
	catalog     Catalog
	running     Running
	sealer      Sealer
	authz       Authorizer
	publisher   Publisher
	requests    Requests
	endpoint    Endpoint
}

func NewDatabasesUsecase(store Store, provisioner Provisioner, catalog Catalog, running Running, sealer Sealer, authz Authorizer, publisher Publisher, requests Requests, endpoint Endpoint) DatabasesUsecase {
	return DatabasesUsecase{
		store: store, provisioner: provisioner, catalog: catalog, running: running,
		sealer: sealer, authz: authz, publisher: publisher, requests: requests, endpoint: endpoint,
	}
}

func (u DatabasesUsecase) Endpoint() Endpoint {
	return u.endpoint
}

func (u DatabasesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Database, authz.Gate, error) {
	db, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Database{}, "", err
	}
	gate, err := u.authz.Gate(ctx, a, authz.EditDatabase, resourceOf(db))
	return db, gate, err
}

func resourceOf(db entities.Database) authz.Resource {
	return authz.Resource{TeamID: db.TeamID, Environment: db.Environment}
}

func (u DatabasesUsecase) publish(ctx context.Context, a actor.Actor, gate authz.Gate, action string, db entities.Database, details map[string]any) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetDatabase,
		TargetID: db.ID, TargetName: db.Name, TeamID: db.TeamID, Environment: db.Environment, Details: events.WithApproval(a, details),
	})
	authz.PublishBypass(ctx, u.publisher, gate, a, authz.EditDatabase, resourceOf(db))
}

func (u DatabasesUsecase) slugsOf(ctx context.Context, serviceIDs []string) (string, error) {
	slugs := make([]string, 0, len(serviceIDs))
	for _, id := range serviceIDs {
		slug, err := u.slugOf(ctx, id)
		if err != nil {
			return "", err
		}
		slugs = append(slugs, slug)
	}
	return strings.Join(slugs, ", "), nil
}

func (u DatabasesUsecase) slugOf(ctx context.Context, serviceID string) (string, error) {
	svc, err := u.catalog.FindServiceByID(ctx, serviceID)
	if apperror.CodeOf(err) == apperror.CodeNotFound {
		return serviceID, nil
	}
	return svc.Slug, err
}

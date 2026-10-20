package databases_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var ana = actor.Actor{UserID: "u-ana", Login: "ana"}

var errBroken = errors.New("broken")

type store struct {
	databases    map[string]entities.Database
	attachments  []entities.Attachment
	failCreate   error
	savedAnyway  bool
	forgotten    []string
	rival        *entities.Database
	failAttachOn string
	failDelete   error
}

func newStore() *store {
	return &store{databases: map[string]entities.Database{}}
}

func (s *store) Create(_ context.Context, d entities.Database) error {
	if s.rival != nil {
		s.databases[s.rival.ID] = *s.rival
	}
	if s.failCreate == nil || s.savedAnyway {
		s.databases[d.ID] = d
	}
	return s.failCreate
}

func (s *store) GetByID(_ context.Context, id string) (entities.Database, error) {
	d, ok := s.databases[id]
	if !ok {
		return entities.Database{}, apperror.NotFound("database not found")
	}
	return d, nil
}

func (s *store) Get(_ context.Context, environment, name string) (entities.Database, error) {
	for _, d := range s.databases {
		if d.Environment == environment && d.Name == name {
			return d, nil
		}
	}
	return entities.Database{}, apperror.NotFound("database not found")
}

func (s *store) List(_ context.Context, environment, teamID string) ([]entities.Database, error) {
	result := []entities.Database{}
	for _, d := range s.databases {
		if (environment == "" || d.Environment == environment) && (teamID == "" || d.TeamID == teamID) {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s *store) Delete(_ context.Context, id string) error {
	if s.failDelete != nil {
		return s.failDelete
	}
	delete(s.databases, id)
	s.attachments = slices.DeleteFunc(s.attachments, func(a entities.Attachment) bool { return a.DatabaseID == id })
	return nil
}

func (s *store) Attach(_ context.Context, a entities.Attachment) error {
	if a.ServiceID == s.failAttachOn {
		return errBroken
	}
	s.attachments = slices.DeleteFunc(s.attachments, func(o entities.Attachment) bool {
		return o.DatabaseID == a.DatabaseID && o.ServiceID == a.ServiceID
	})
	s.attachments = append(s.attachments, a)
	return nil
}

func (s *store) Detach(_ context.Context, databaseID, serviceID string, at time.Time) error {
	for i, a := range s.attachments {
		if a.DatabaseID == databaseID && a.ServiceID == serviceID {
			s.attachments[i].DetachedAt = &at
		}
	}
	return nil
}

func (s *store) Attachments(_ context.Context, databaseID string) ([]entities.Attachment, error) {
	result := []entities.Attachment{}
	for _, a := range s.attachments {
		if a.DatabaseID == databaseID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *store) ActiveFor(_ context.Context, serviceID, environment string) (entities.Database, bool, error) {
	for _, a := range s.attachments {
		if a.ServiceID == serviceID && a.Environment == environment && a.Active() {
			return s.databases[a.DatabaseID], true, nil
		}
	}
	return entities.Database{}, false, nil
}

func (s *store) ForgetDrained(_ context.Context, databaseID, serviceID string) error {
	s.forgotten = append(s.forgotten, serviceID)
	s.attachments = slices.DeleteFunc(s.attachments, func(a entities.Attachment) bool {
		return a.DatabaseID == databaseID && a.ServiceID == serviceID && !a.Active()
	})
	return nil
}

func (s *store) DetachService(_ context.Context, serviceID string, at time.Time) error {
	for i, a := range s.attachments {
		if a.ServiceID == serviceID && a.Active() {
			s.attachments[i].DetachedAt = &at
		}
	}
	return nil
}

type created struct {
	schema, user, password string
	maxConnections         int
}

type provisioner struct {
	existing   map[string]bool
	created    []created
	dropped    []string
	reads      int
	writes     int
	failCreate error
	adopted    []created
	released   []string
	failAdopt  error
}

func (p *provisioner) SchemaExists(_ context.Context, schema string) (bool, error) {
	p.reads++
	return p.existing[schema], nil
}

func (p *provisioner) Create(_ context.Context, schema, user, password string, maxConnections int) error {
	p.writes++
	if p.failCreate != nil {
		return p.failCreate
	}
	p.created = append(p.created, created{schema, user, password, maxConnections})
	return nil
}

func (p *provisioner) Drop(_ context.Context, schema, user string) error {
	p.writes++
	p.dropped = append(p.dropped, schema+"/"+user)
	return nil
}

func (p *provisioner) Adopt(_ context.Context, schema, user, password string, maxConnections int) error {
	p.writes++
	if p.failAdopt != nil {
		return p.failAdopt
	}
	p.adopted = append(p.adopted, created{schema, user, password, maxConnections})
	return nil
}

func (p *provisioner) Release(_ context.Context, user string) error {
	p.writes++
	p.released = append(p.released, user)
	return nil
}

var services = map[string]databases.ServiceRef{
	"api":      {ID: "s-api", Slug: "api", OwnerTeamID: "t-plateful"},
	"importer": {ID: "s-importer", Slug: "importer", OwnerTeamID: "t-plateful"},
	"shop":     {ID: "s-shop", Slug: "shop", OwnerTeamID: "t-plateful"},
	"worker":   {ID: "s-worker", Slug: "worker", OwnerTeamID: "t-plateful"},
	"other":    {ID: "s-other", Slug: "other", OwnerTeamID: "t-reviews"},
}

type catalog struct{}

func (catalog) FindService(_ context.Context, slug string) (databases.ServiceRef, error) {
	s, ok := services[slug]
	if !ok {
		return databases.ServiceRef{}, apperror.NotFound("service not found")
	}
	return s, nil
}

func (catalog) FindServiceByID(_ context.Context, id string) (databases.ServiceRef, error) {
	for _, s := range services {
		if s.ID == id {
			return s, nil
		}
	}
	return databases.ServiceRef{}, apperror.NotFound("service not found")
}

func (catalog) FindTeam(_ context.Context, slug string) (string, error) {
	return "t-" + slug, nil
}

func (catalog) TeamSlug(_ context.Context, id string) (string, error) {
	return id[2:], nil
}

func (catalog) IsProtected(_ context.Context, environment string) (bool, error) {
	switch environment {
	case "dev":
		return false, nil
	case "prod":
		return true, nil
	}
	return false, apperror.NotFound("environment not found")
}

type running map[string]bool

func (r running) RunsSince(_ context.Context, serviceID, _ string, _ time.Time) (bool, error) {
	return r[serviceID], nil
}

type policy struct {
	gate   authz.Gate
	denied map[authz.Action]string
}

func (p policy) Gate(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error) {
	if team, ok := p.denied[action]; ok && (team == "" || team == r.TeamID) {
		return "", apperror.Forbidden("no")
	}
	if p.gate == "" {
		return authz.Open, nil
	}
	return p.gate, nil
}

type requests struct{ changes []approval.Change }

func (r *requests) Submit(_ context.Context, _ actor.Actor, c approval.Change) (*approval.Pending, error) {
	r.changes = append(r.changes, c)
	return &approval.Pending{ID: "c1", Status: "pending"}, nil
}

type recorder struct{ events []events.Event }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.events = append(r.events, e)
}

func (r *recorder) actions() []string {
	result := []string{}
	for _, e := range r.events {
		result = append(result, e.Action)
	}
	return result
}

type fixture struct {
	store       *store
	provisioner *provisioner
	running     running
	sealer      *secretbox.Box
	requests    *requests
	events      *recorder
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	box, err := secretbox.NewWithKey("k1", make([]byte, 32))
	require.NoError(t, err)
	return fixture{
		store: newStore(), provisioner: &provisioner{existing: map[string]bool{}}, running: running{},
		sealer: box, requests: &requests{}, events: &recorder{},
	}
}

func (f fixture) with(p policy) databases.DatabasesUsecase {
	return f.at(p, databases.Endpoint{Host: "mysql", Port: 3306})
}

func (f fixture) at(p policy, endpoint databases.Endpoint) databases.DatabasesUsecase {
	return databases.NewDatabasesUsecase(f.store, f.provisioner, catalog{}, f.running, f.sealer, p, f.events, f.requests, endpoint)
}

func (f fixture) open() databases.DatabasesUsecase {
	return f.with(policy{})
}

func (f fixture) as(gate authz.Gate) databases.DatabasesUsecase {
	return f.with(policy{gate: gate})
}

func shopDraft() databases.Draft {
	return databases.Draft{Name: "shop", Environment: "dev", Team: "plateful"}
}

func (f fixture) created(t *testing.T, d databases.Draft) entities.Database {
	t.Helper()
	db, err := f.open().Create(ctx, ana, d)
	require.NoError(t, err)
	return db
}

func (f fixture) attached(t *testing.T, environment, name string, slugs ...string) {
	t.Helper()
	for _, slug := range slugs {
		_, err := f.open().Attach(ctx, ana, environment, name, slug)
		require.NoError(t, err)
	}
}

func hintOf(t *testing.T, err error) string {
	t.Helper()
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	return appErr.Hint
}

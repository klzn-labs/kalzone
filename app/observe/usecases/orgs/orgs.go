package orgs

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

type Grafana interface {
	Enabled() bool
	AdminLogin() string
	Orgs(ctx context.Context) ([]entities.Org, error)
	CreateOrg(ctx context.Context, name string) (int, error)
	DeleteOrg(ctx context.Context, id int) error
	EnsureUser(ctx context.Context, p entities.Person) error
	Members(ctx context.Context, org int) ([]entities.OrgMember, error)
	AddMember(ctx context.Context, org int, login, role string) error
	SetRole(ctx context.Context, org, userID int, role string) error
	RemoveMember(ctx context.Context, org, userID int) error
	UpsertDatasource(ctx context.Context, org int, d entities.Datasource) error
	EnsureFolder(ctx context.Context, org int) error
	UpsertDashboard(ctx context.Context, org int, dashboard map[string]any) error
	DeleteDashboard(ctx context.Context, org int, uid string) error
	DashboardUIDs(ctx context.Context, org int) ([]string, error)
}

type Roster interface {
	Roster(ctx context.Context) ([]entities.OrgTeam, []entities.Person, error)
}

type Simulated interface {
	Simulated(ctx context.Context) ([]entities.Service, error)
}

type Tokens interface {
	Enabled() bool
	Token(team string) string
}

type OrgsUsecase struct {
	grafana     Grafana
	roster      Roster
	simulated   Simulated
	tokens      Tokens
	internalURL string
	known       *knownOrgs
}

type knownOrgs struct {
	mu     sync.RWMutex
	byName map[string]int
}

func NewOrgsUsecase(grafana Grafana, roster Roster, simulated Simulated, tokens Tokens, internalURL string) OrgsUsecase {
	return OrgsUsecase{
		grafana: grafana, roster: roster, simulated: simulated, tokens: tokens,
		internalURL: strings.TrimRight(internalURL, "/"), known: &knownOrgs{byName: map[string]int{}},
	}
}

func (u OrgsUsecase) Enabled() bool {
	return u.grafana.Enabled() && u.tokens.Enabled()
}

func (u OrgsUsecase) OrgID(name string) (int, bool) {
	u.known.mu.RLock()
	defer u.known.mu.RUnlock()
	id, ok := u.known.byName[name]
	return id, ok
}

func (u OrgsUsecase) remember(ids map[string]int) {
	u.known.mu.Lock()
	defer u.known.mu.Unlock()
	u.known.byName = ids
}

func (u OrgsUsecase) Sync(ctx context.Context, services []entities.Service) error {
	if !u.Enabled() {
		return nil
	}
	in, err := u.inputs(ctx, services)
	if err != nil {
		return err
	}
	existing, err := u.grafana.Orgs(ctx)
	if err != nil {
		return err
	}
	ids := idsByName(existing)
	plans := entities.OrgPlans(in)
	errs := []error{u.ensureUsers(ctx, in.People), u.syncMainOrg(ctx, entities.MainOrgRoles(in.People))}
	for _, plan := range plans {
		errs = append(errs, u.apply(ctx, ids, plan))
	}
	errs = append(errs, u.prune(ctx, existing, plans, ids))
	u.remember(ids)
	return errors.Join(errs...)
}

func (u OrgsUsecase) inputs(ctx context.Context, services []entities.Service) (entities.OrgInputs, error) {
	teams, people, err := u.roster.Roster(ctx)
	if err != nil {
		return entities.OrgInputs{}, err
	}
	simulated, err := u.simulated.Simulated(ctx)
	tokens := map[string]string{}
	for _, t := range teams {
		tokens[t.Slug] = u.tokens.Token(t.Slug)
	}
	link := entities.TelemetryLink{URL: u.internalURL, Tokens: tokens}
	return entities.OrgInputs{Teams: teams, People: people, Services: services, Simulated: simulated, Telemetry: link}, err
}

func idsByName(orgs []entities.Org) map[string]int {
	ids := map[string]int{}
	for _, o := range orgs {
		ids[o.Name] = o.ID
	}
	return ids
}

func (u OrgsUsecase) ensureUsers(ctx context.Context, people []entities.Person) error {
	errs := []error{}
	for _, p := range people {
		errs = append(errs, u.grafana.EnsureUser(ctx, p))
	}
	return errors.Join(errs...)
}

package seed

import (
	"context"
	"errors"
	"time"

	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/demo"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type Teams interface {
	FindBySlug(ctx context.Context, slug string) (team.Team, error)
	Create(ctx context.Context, t team.Team) error
	PutMembership(ctx context.Context, m team.Membership) error
}

type Users interface {
	Save(ctx context.Context, user identity.User) error
}

type Services interface {
	Create(ctx context.Context, s catalog.Service) error
}

type Configs interface {
	Insert(ctx context.Context, rev envconfig.Revision) error
}

type Releases interface {
	Create(ctx context.Context, rel release.Release) (release.Release, error)
}

type Changelogs interface {
	Save(ctx context.Context, c release.Changelog) error
}

type Deployments interface {
	Create(ctx context.Context, d deploy.Deployment) (deploy.Deployment, error)
}

type Requests interface {
	Create(ctx context.Context, r approvals.Request) error
}

type Events interface {
	Insert(ctx context.Context, e audit.Event) error
}

type Shifter interface {
	Shift(ctx context.Context, teamID string, by time.Duration) error
}

type Store struct {
	Teams       Teams
	Users       Users
	Services    Services
	Configs     Configs
	Releases    Releases
	Changelogs  Changelogs
	Deployments Deployments
	Requests    Requests
	Events      Events
	Shifter     Shifter
}

type SeedUsecase struct {
	store   Store
	enabled bool
}

func NewSeedUsecase(cfg config.Config, store Store) SeedUsecase {
	return SeedUsecase{store: store, enabled: cfg.Demo.Enabled}
}

func (u SeedUsecase) Seed(ctx context.Context) error {
	if !u.enabled {
		return nil
	}
	_, err := u.store.Teams.FindBySlug(ctx, demo.TeamSlug)
	if !errors.Is(err, apperror.NotFound("")) {
		return err
	}
	return u.plant(ctx, clock.Now().UTC().Truncate(time.Second))
}

func (u SeedUsecase) Sweep(ctx context.Context) (int, error) {
	if !u.enabled {
		return 0, nil
	}
	t, err := u.store.Teams.FindBySlug(ctx, demo.TeamSlug)
	if err != nil {
		return 0, err
	}
	drift := clock.Now().Sub(t.CreatedAt.Add(teamAge)).Truncate(time.Second)
	if drift < refreshAfter {
		return 0, nil
	}
	return 1, u.store.Shifter.Shift(ctx, t.ID, drift)
}

func (u SeedUsecase) plant(ctx context.Context, now time.Time) error {
	teamID := uuid.NewUUID()
	if err := u.team(ctx, teamID, now); err != nil {
		return err
	}
	for _, w := range workloads {
		if err := u.workload(ctx, teamID, w, now); err != nil {
			return err
		}
	}
	return nil
}

func (u SeedUsecase) team(ctx context.Context, teamID string, now time.Time) error {
	if err := u.store.Teams.Create(ctx, team.Team{ID: teamID, Slug: demo.TeamSlug, Name: demo.TeamName, Demo: true, CreatedAt: now.Add(-teamAge)}); err != nil {
		return err
	}
	guest := identity.User{ID: demo.GuestID, Kind: identity.KindGuest, Login: demo.GuestLogin, Name: demo.GuestName, CreatedAt: now, LastLoginAt: now}
	if err := u.store.Users.Save(ctx, guest); err != nil {
		return err
	}
	for _, p := range people {
		if err := u.person(ctx, teamID, p, now); err != nil {
			return err
		}
	}
	return nil
}

func (u SeedUsecase) person(ctx context.Context, teamID string, p person, now time.Time) error {
	user := identity.User{ID: p.id, Kind: identity.KindPersona, Login: p.login, Name: p.name, CreatedAt: now.Add(-30 * day), LastLoginAt: now}
	if err := u.store.Users.Save(ctx, user); err != nil {
		return err
	}
	return u.store.Teams.PutMembership(ctx, team.Membership{TeamID: teamID, UserID: p.id, Role: p.role, CreatedAt: now.Add(-30 * day)})
}

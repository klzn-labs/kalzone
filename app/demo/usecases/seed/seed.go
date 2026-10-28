package seed

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type Teams interface {
	FindBySlug(ctx context.Context, slug string) (team.Team, error)
	Create(ctx context.Context, t team.Team) error
	PutMembership(ctx context.Context, m team.Membership) error
	MembershipsOfTeam(ctx context.Context, teamID string) ([]team.Membership, error)
}

type Users interface {
	Save(ctx context.Context, user identity.User) error
	FindByLogin(ctx context.Context, login string) (identity.User, error)
}

type Services interface {
	Create(ctx context.Context, s catalog.Service) error
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
	Update(ctx context.Context, s catalog.Service) error
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

type Wiper interface {
	Wipe(ctx context.Context, teamID string) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
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
	Wiper       Wiper
}

type SeedUsecase struct {
	store     Store
	publisher Publisher
	enabled   bool
	member    string
}

func NewSeedUsecase(cfg config.Config, store Store, publisher Publisher) SeedUsecase {
	return SeedUsecase{store: store, publisher: publisher, enabled: cfg.Demo.Enabled, member: cfg.Demo.Member}
}

func (u SeedUsecase) Seed(ctx context.Context) error {
	if !u.enabled {
		return nil
	}
	member, err := u.store.Users.FindByLogin(ctx, u.member)
	if err != nil {
		return fmt.Errorf("demo member %q: %w", u.member, err)
	}
	t, err := u.store.Teams.FindBySlug(ctx, demo.TeamSlug)
	if errors.Is(err, apperror.NotFound("")) {
		return u.plant(ctx, member, clock.Now().UTC().Truncate(time.Second))
	}
	if err != nil {
		return err
	}
	return u.refresh(ctx, t.ID, member)
}

func (u SeedUsecase) refresh(ctx context.Context, teamID string, member identity.User) error {
	members, err := u.store.Teams.MembershipsOfTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if len(members) == 1 && members[0].UserID == member.ID {
		return u.prune(ctx, teamID)
	}
	if err := u.store.Wiper.Wipe(ctx, teamID); err != nil {
		return err
	}
	return u.plant(ctx, member, clock.Now().UTC().Truncate(time.Second))
}

func (u SeedUsecase) prune(ctx context.Context, teamID string) error {
	services, err := u.store.Services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{teamID}})
	if err != nil {
		return err
	}
	now := clock.Now().UTC()
	for _, s := range services {
		if planted(s.Slug) {
			continue
		}
		s.DeletedAt = &now
		if err := u.store.Services.Update(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func planted(slug string) bool {
	for _, w := range workloads {
		if w.slug == slug {
			return true
		}
	}
	return false
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

func (u SeedUsecase) plant(ctx context.Context, member identity.User, now time.Time) error {
	teamID := uuid.NewUUID()
	if err := u.team(ctx, teamID, member, now); err != nil {
		return err
	}
	for _, w := range workloads {
		if err := u.workload(ctx, teamID, member, w, now); err != nil {
			return err
		}
	}
	u.publisher.Publish(ctx, events.Event{
		At: now, ActorID: member.ID, ActorLogin: member.Login, Action: "team.created",
		TargetKind: events.TargetTeam, TargetID: teamID, TargetName: demo.TeamSlug, TeamID: teamID,
	})
	return nil
}

func (u SeedUsecase) team(ctx context.Context, teamID string, member identity.User, now time.Time) error {
	if err := u.store.Teams.Create(ctx, team.Team{ID: teamID, Slug: demo.TeamSlug, Name: demo.TeamName, Demo: true, CreatedAt: now.Add(-teamAge)}); err != nil {
		return err
	}
	guest := identity.User{ID: demo.GuestID, Kind: identity.KindGuest, Login: demo.GuestLogin, Name: demo.GuestName, CreatedAt: now, LastLoginAt: now}
	if err := u.store.Users.Save(ctx, guest); err != nil {
		return err
	}
	return u.store.Teams.PutMembership(ctx, team.Membership{TeamID: teamID, UserID: member.ID, Role: team.RoleMaintainer, CreatedAt: now.Add(-teamAge)})
}

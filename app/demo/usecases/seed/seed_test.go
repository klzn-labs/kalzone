package seed_test

import (
	"context"
	"testing"
	"time"

	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/demo/usecases/seed"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/demo"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

type store struct {
	teams       []team.Team
	members     []team.Membership
	users       []identity.User
	services    []catalog.Service
	revisions   []envconfig.Revision
	releases    []release.Release
	changelogs  []release.Changelog
	deployments []deploy.Deployment
	requests    []approvals.Request
	events      []audit.Event
	shifts      []time.Duration
}

func (s *store) FindBySlug(_ context.Context, slug string) (team.Team, error) {
	for _, t := range s.teams {
		if t.Slug == slug {
			return t, nil
		}
	}
	return team.Team{}, apperror.NotFound("team not found")
}

func (s *store) Create(_ context.Context, t team.Team) error {
	s.teams = append(s.teams, t)
	return nil
}
func (s *store) PutMembership(_ context.Context, m team.Membership) error {
	s.members = append(s.members, m)
	return nil
}
func (s *store) Save(_ context.Context, u identity.User) error {
	s.users = append(s.users, u)
	return nil
}

type services struct{ s *store }

func (f services) Create(_ context.Context, s catalog.Service) error {
	f.s.services = append(f.s.services, s)
	return nil
}

type configs struct{ s *store }

func (f configs) Insert(_ context.Context, r envconfig.Revision) error {
	f.s.revisions = append(f.s.revisions, r)
	return nil
}

type releases struct{ s *store }

func (f releases) Create(_ context.Context, r release.Release) (release.Release, error) {
	r.Number = 1
	for _, existing := range f.s.releases {
		if existing.ServiceID == r.ServiceID {
			r.Number++
		}
	}
	f.s.releases = append(f.s.releases, r)
	return r, nil
}

type changelogs struct{ s *store }

func (f changelogs) Save(_ context.Context, c release.Changelog) error {
	f.s.changelogs = append(f.s.changelogs, c)
	return nil
}

type deployments struct{ s *store }

func (f deployments) Create(_ context.Context, d deploy.Deployment) (deploy.Deployment, error) {
	d.Number = len(f.s.deployments) + 1
	f.s.deployments = append(f.s.deployments, d)
	return d, nil
}

type shifter struct{ s *store }

func (f shifter) Shift(_ context.Context, teamID string, by time.Duration) error {
	for i := range f.s.teams {
		if f.s.teams[i].ID == teamID {
			f.s.teams[i].CreatedAt = f.s.teams[i].CreatedAt.Add(by)
		}
	}
	f.s.shifts = append(f.s.shifts, by)
	return nil
}

type requests struct{ s *store }

func (f requests) Create(_ context.Context, r approvals.Request) error {
	f.s.requests = append(f.s.requests, r)
	return nil
}

type events struct{ s *store }

func (f events) Insert(_ context.Context, e audit.Event) error {
	f.s.events = append(f.s.events, e)
	return nil
}

func seeder(enabled bool) (*store, seed.SeedUsecase) {
	clock.WithTime(now)
	s := &store{}
	deps := seed.Store{
		Teams: s, Users: s, Services: services{s}, Configs: configs{s}, Releases: releases{s}, Changelogs: changelogs{s},
		Deployments: deployments{s}, Requests: requests{s}, Events: events{s}, Shifter: shifter{s},
	}
	return s, seed.NewSeedUsecase(config.Config{Demo: config.Demo{Enabled: enabled}}, deps)
}

func Test_Seed_DoesNothingWhileTheDemoIsOff(t *testing.T) {
	s, u := seeder(false)

	require.NoError(t, u.Seed(context.Background()))

	require.Empty(t, s.teams)
}

func Test_Seed_PlantsASimulatedDemoTeamOnce(t *testing.T) {
	s, u := seeder(true)

	require.NoError(t, u.Seed(context.Background()))
	require.NoError(t, u.Seed(context.Background()))

	require.Len(t, s.teams, 1)
	require.True(t, s.teams[0].Demo)
	require.Equal(t, demo.GuestID, s.users[0].ID)
	require.True(t, s.users[0].Guest())
	require.Len(t, s.members, 3)
	require.Len(t, s.services, 4)
	require.Len(t, s.revisions, 8)
	for _, svc := range s.services {
		require.True(t, svc.Simulated, svc.Slug)
	}
}

func Test_Seed_LeavesOnlyFinishedDeploysAndOneWaitingRequest(t *testing.T) {
	s, u := seeder(true)

	require.NoError(t, u.Seed(context.Background()))

	for _, d := range s.deployments {
		require.False(t, d.Active(), d.ID)
		require.False(t, d.FinishedAt.After(now), d.ID)
	}
	require.Len(t, s.requests, 1)
	require.Equal(t, "deploy release #18 to prod", s.requests[0].Summary)
	require.True(t, s.requests[0].ExpiresAt.After(now.Add(7*24*time.Hour)))
	require.Contains(t, triggers(s.deployments), deploy.TriggerRollback)
	require.NotEmpty(t, s.changelogs)
	require.NotEmpty(t, s.events)
}

func triggers(list []deploy.Deployment) []deploy.Trigger {
	result := []deploy.Trigger{}
	for _, d := range list {
		result = append(result, d.Trigger)
	}
	return result
}

func Test_Sweep_KeepsTheDemoAsFreshAsTheDayItWasSeeded(t *testing.T) {
	s, u := seeder(true)
	require.NoError(t, u.Seed(context.Background()))

	clock.WithTime(now.Add(30 * time.Minute))
	early, err := u.Sweep(context.Background())
	require.NoError(t, err)
	clock.WithTime(now.Add(26 * time.Hour))
	late, _ := u.Sweep(context.Background())
	again, _ := u.Sweep(context.Background())

	require.Zero(t, early)
	require.Equal(t, 1, late)
	require.Zero(t, again)
	require.Equal(t, []time.Duration{26 * time.Hour}, s.shifts)
}

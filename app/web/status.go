package web

import (
	"context"

	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	database "github.com/klzn-labs/kalzone/app/database/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	observe "github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/health"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const (
	releasesRead = 4
	deploysRead  = 30
)

type ReleaseLister interface {
	List(ctx context.Context, serviceSlug string, limit int) ([]release.Release, error)
}

type DeploymentHistory interface {
	History(ctx context.Context, serviceSlug, environment string, limit int) ([]deploy.Deployment, error)
}

type AttachmentReader interface {
	AttachedTo(ctx context.Context, serviceID string) ([]database.Database, error)
}

type HealthReader interface {
	Day(ctx context.Context, t health.Target) observe.Health
	NowAll(ctx context.Context, targets []health.Target) map[string]observe.Health
}

type AwaitingReader interface {
	Awaiting(ctx context.Context, a actor.Actor) ([]approvals.Request, error)
}

type StatusSources struct {
	Releases    ReleaseLister
	Deployments DeploymentHistory
	Configs     ConfigReader
	Databases   AttachmentReader
	Health      HealthReader
	Requests    AwaitingReader
}

type Statuses struct {
	sources StatusSources
}

func NewStatuses(sources StatusSources) Statuses {
	return Statuses{sources: sources}
}

type ServiceStatus struct {
	Service  catalog.Service
	Releases []release.Release
	Deploys  []deploy.Deployment
	Waiting  *approvals.Request
	Health   observe.Health
	Problems []string
	target   health.Target
}

func (s Statuses) Now(ctx context.Context, a actor.Actor, services []catalog.Service, prod string) ([]ServiceStatus, error) {
	statuses, err := s.collect(ctx, a, services, prod)
	if err != nil {
		return nil, err
	}
	targets := make([]health.Target, 0, len(statuses))
	for _, st := range statuses {
		targets = append(targets, st.target)
	}
	now := s.sources.Health.NowAll(ctx, targets)
	for i := range statuses {
		statuses[i].judge(now[statuses[i].Service.Slug])
	}
	return statuses, nil
}

func (s Statuses) Day(ctx context.Context, a actor.Actor, service catalog.Service, prod string) (ServiceStatus, error) {
	statuses, err := s.collect(ctx, a, []catalog.Service{service}, prod)
	if err != nil {
		return ServiceStatus{}, err
	}
	st := statuses[0]
	st.judge(s.sources.Health.Day(ctx, st.target))
	return st, nil
}

func (s Statuses) collect(ctx context.Context, a actor.Actor, services []catalog.Service, prod string) ([]ServiceStatus, error) {
	waiting, err := s.sources.Requests.Awaiting(ctx, a)
	if err != nil {
		return nil, err
	}
	statuses := make([]ServiceStatus, 0, len(services))
	for _, service := range services {
		st, err := s.one(ctx, service, prod)
		if err != nil {
			return nil, err
		}
		st.Waiting = waitingFor(waiting, service.Slug, prod)
		statuses = append(statuses, st)
	}
	return statuses, nil
}

func (s Statuses) one(ctx context.Context, service catalog.Service, prod string) (ServiceStatus, error) {
	st := ServiceStatus{Service: service}
	var err error
	if st.Releases, err = s.sources.Releases.List(ctx, service.Slug, releasesRead); err != nil {
		return st, err
	}
	if st.Deploys, err = s.sources.Deployments.History(ctx, service.Slug, prod, deploysRead); err != nil {
		return st, err
	}
	st.target, err = s.target(ctx, service, prod)
	return st, err
}

func (s Statuses) target(ctx context.Context, service catalog.Service, prod string) (health.Target, error) {
	rev, err := s.sources.Configs.Current(ctx, service.Slug, prod)
	if err != nil {
		return health.Target{}, err
	}
	dbs, err := s.sources.Databases.AttachedTo(ctx, service.ID)
	t := health.Target{Slug: service.Slug, CPUMillis: rev.Spec.CPUMillis, Limits: service.Limits}
	for _, db := range dbs {
		t.Database = t.Database || db.Environment == prod
	}
	return t, err
}

func waitingFor(requests []approvals.Request, slug, prod string) *approvals.Request {
	for _, r := range requests {
		if r.ServiceSlug == slug && r.Environment == prod {
			return &r
		}
	}
	return nil
}

func (st *ServiceStatus) judge(h observe.Health) {
	st.Health = h
	st.Problems = observe.Problems(h, st.delivery())
}

func (st ServiceStatus) delivery() observe.Delivery {
	d := observe.Delivery{}
	if latest := st.Latest(); latest != nil && latest.Status == release.StatusFailed {
		d.FailedBuild = latest.Number
	}
	if last := st.lastFinished(); last != nil && last.Status == deploy.StatusFailed {
		d.FailedDeploy = last.Number
	}
	return d
}

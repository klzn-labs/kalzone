package changelogs

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type Store interface {
	Save(ctx context.Context, c entities.Changelog) error
	Find(ctx context.Context, releaseID string) (entities.Changelog, bool, error)
	FindMany(ctx context.Context, releaseIDs []string) (map[string]entities.Changelog, error)
	Pending(ctx context.Context) ([]entities.Changelog, error)
	ByPair(ctx context.Context, baseSHA, headSHA string) (entities.Changelog, bool, error)
}

type Releases interface {
	FindByID(ctx context.Context, id string) (entities.Release, error)
	PreviousBuilt(ctx context.Context, serviceID, excludeID string, finishedBefore time.Time) (entities.Release, bool, error)
	ListByService(ctx context.Context, serviceID string, before, limit int) ([]entities.Release, error)
}

type Services interface {
	FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error)
}

type GitHub interface {
	Compare(ctx context.Context, repository, base, head string) (entities.Comparison, error)
	Commit(ctx context.Context, repository, sha string) (entities.Commit, error)
	PullFor(ctx context.Context, repository, sha string) (*entities.PullRef, error)
}

type ChangelogsUsecase struct {
	store    Store
	releases Releases
	services Services
	github   GitHub
	run      func(func())
	flights  *flights
}

func NewChangelogsUsecase(store Store, releases Releases, services Services, github GitHub) ChangelogsUsecase {
	return ChangelogsUsecase{store: store, releases: releases, services: services, github: github, run: inBackground, flights: newFlights()}
}

func inBackground(f func()) {
	go f()
}

func (u ChangelogsUsecase) WithRunner(run func(func())) ChangelogsUsecase {
	u.run = run
	return u
}

func (u ChangelogsUsecase) Handle(ctx context.Context, e events.Event) error {
	if e.TargetKind != events.TargetRelease || (e.Action != "release.built" && e.Action != "release.shared") {
		return nil
	}
	id, _ := e.Details["release_id"].(string)
	rel, err := u.releases.FindByID(ctx, id)
	if err != nil || rel.CommitSHA == "" {
		return err
	}
	pending := entities.Changelog{ReleaseID: rel.ID, HeadSHA: rel.CommitSHA, State: entities.ChangelogPending, UpdatedAt: now()}
	if err := u.store.Save(ctx, pending); err != nil {
		return err
	}
	background := context.WithoutCancel(ctx)
	u.run(func() { u.logFill(background, rel.ID) })
	return nil
}

const errBeforeChangelogs = "built before changelogs existed"

func (u ChangelogsUsecase) Get(ctx context.Context, rel entities.Release) (entities.Changelog, error) {
	if rel.CommitSHA == "" {
		return entities.Changelog{ReleaseID: rel.ID, State: entities.ChangelogUnavailable}, nil
	}
	c, found, err := u.store.Find(ctx, rel.ID)
	if err != nil || !found {
		return entities.Changelog{ReleaseID: rel.ID, HeadSHA: rel.CommitSHA, State: entities.ChangelogUnavailable, LastError: errBeforeChangelogs}, err
	}
	return u.withBaseNumber(ctx, c)
}

func (u ChangelogsUsecase) withBaseNumber(ctx context.Context, c entities.Changelog) (entities.Changelog, error) {
	if c.BaseReleaseID == "" {
		return c, nil
	}
	base, err := u.releases.FindByID(ctx, c.BaseReleaseID)
	c.BaseNumber = base.Number
	return c, err
}

func (u ChangelogsUsecase) Many(ctx context.Context, releaseIDs []string) (map[string]entities.Changelog, error) {
	return u.store.FindMany(ctx, releaseIDs)
}

func now() time.Time {
	return clock.Now().UTC().Truncate(time.Millisecond)
}

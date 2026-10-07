package releases

import (
	"context"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
)

var errAlreadyFinal = apperror.Conflict("this release is already finished")

type pending struct {
	service entities.ServiceRef
	release entities.Release
}

func (u ReleasesUsecase) Started(ctx context.Context, caller entities.BuildCaller, releaseID, runURL string) (entities.Release, error) {
	p, err := u.pendingFor(ctx, caller, releaseID)
	if err != nil {
		return entities.Release{}, err
	}
	rel := p.release
	from := rel.Status
	rel.Status, rel.RunURL = entities.StatusBuilding, truncate(runURL, 500)
	return rel, u.store.Transition(ctx, rel, from)
}

func (u ReleasesUsecase) Built(ctx context.Context, caller entities.BuildCaller, releaseID string, report entities.BuildReport) (entities.Release, error) {
	if err := report.Validate(); err != nil {
		return entities.Release{}, err
	}
	p, err := u.pendingFor(ctx, caller, releaseID)
	if err != nil {
		return entities.Release{}, err
	}
	rel := p.release
	from := rel.Status
	now := clock.Now().UTC()
	rel.Status, rel.Digest, rel.CommitSHA, rel.Tag, rel.FinishedAt = entities.StatusBuilt, report.Digest, report.CommitSHA, report.Tag, &now
	rel.RunURL = firstNonEmpty(truncate(report.RunURL, 500), rel.RunURL)
	if err := u.store.Transition(ctx, rel, from); err != nil {
		return entities.Release{}, err
	}
	u.publish(ctx, GitHubActor, "release.built", p.service, rel)
	return rel, nil
}

func (u ReleasesUsecase) Failed(ctx context.Context, caller entities.BuildCaller, releaseID string, report entities.BuildReport) (entities.Release, error) {
	p, err := u.pendingFor(ctx, caller, releaseID)
	if err != nil {
		return entities.Release{}, err
	}
	rel := p.release
	rel.RunURL = firstNonEmpty(truncate(report.RunURL, 500), rel.RunURL)
	return u.fail(ctx, GitHubActor, p.service, rel, firstNonEmpty(report.Failure, "the build failed"))
}

func (u ReleasesUsecase) pendingFor(ctx context.Context, caller entities.BuildCaller, releaseID string) (pending, error) {
	rel, err := u.store.FindByID(ctx, releaseID)
	if err != nil {
		return pending{}, err
	}
	service, err := u.services.FindServiceByID(ctx, rel.ServiceID)
	if err != nil {
		return pending{}, err
	}
	if service.GitHubRepo == "" || service.GitHubRepo != caller.Repository {
		return pending{}, errNotYours
	}
	if rel.Final() {
		return pending{}, errAlreadyFinal
	}
	return pending{service: service, release: rel}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

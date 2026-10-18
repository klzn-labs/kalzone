package releases

import (
	"context"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const commitCandidates = 100

var errAmbiguousCommit = apperror.Invalid("that commit prefix matches more than one commit").
	WithHint("send more characters of the commit, up to the full 40")

func (u ReleasesUsecase) Resolve(ctx context.Context, serviceSlug, ref string) (entities.Release, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.Release{}, err
	}
	return u.resolveIn(ctx, service, ref)
}

func (u ReleasesUsecase) ResolveFor(ctx context.Context, a actor.Actor, serviceSlug, ref string) (entities.Release, error) {
	service, err := u.visibleService(ctx, a, serviceSlug)
	if err != nil {
		return entities.Release{}, err
	}
	return u.resolveIn(ctx, service, ref)
}

func (u ReleasesUsecase) resolveIn(ctx context.Context, service entities.ServiceRef, raw string) (entities.Release, error) {
	ref, err := entities.ParseReference(raw)
	if err != nil {
		return entities.Release{}, err
	}
	rel, found, err := u.lookup(ctx, service.ID, ref)
	if err == nil && !found {
		return entities.Release{}, notFoundIn(service, raw)
	}
	return rel, err
}

func (u ReleasesUsecase) lookup(ctx context.Context, serviceID string, ref entities.Reference) (entities.Release, bool, error) {
	switch ref.Kind {
	case entities.ByID:
		return u.byID(ctx, serviceID, ref.Raw)
	case entities.ByLatest:
		return u.store.LatestBuilt(ctx, serviceID)
	case entities.ByNumber:
		return u.store.ByNumber(ctx, serviceID, ref.Number)
	case entities.ByCommit:
		return u.byCommit(ctx, serviceID, ref.Prefix)
	}
	return u.store.BuiltByTag(ctx, serviceID, ref.Raw)
}

func (u ReleasesUsecase) byID(ctx context.Context, serviceID, id string) (entities.Release, bool, error) {
	rel, err := u.store.FindByID(ctx, id)
	if apperror.CodeOf(err) == apperror.CodeNotFound {
		return entities.Release{}, false, nil
	}
	return rel, err == nil && rel.ServiceID == serviceID, err
}

func (u ReleasesUsecase) byCommit(ctx context.Context, serviceID, prefix string) (entities.Release, bool, error) {
	matches, err := u.store.BuiltByCommitPrefix(ctx, serviceID, prefix, commitCandidates)
	if err != nil || len(matches) == 0 {
		return entities.Release{}, false, err
	}
	for _, rel := range matches[1:] {
		if rel.CommitSHA != matches[0].CommitSHA {
			return entities.Release{}, false, errAmbiguousCommit
		}
	}
	return matches[0], true, nil
}

func notFoundIn(service entities.ServiceRef, raw string) error {
	return apperror.NotFound("no release of " + service.Slug + " matches " + raw).
		WithHint("list releases with GET /api/v1/releases?service=" + service.Slug)
}

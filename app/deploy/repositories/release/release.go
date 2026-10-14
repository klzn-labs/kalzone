package release

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	release "github.com/adrianozp/platform/app/release/entities"
)

type ReleaseFinder interface {
	Get(ctx context.Context, id string) (release.Release, error)
	Resolve(ctx context.Context, serviceSlug, ref string) (release.Release, error)
}

type Lookup struct {
	releases ReleaseFinder
}

func NewLookup(releases ReleaseFinder) Lookup {
	return Lookup{releases: releases}
}

func (l Lookup) FindRelease(ctx context.Context, id string) (entities.ReleaseRef, error) {
	r, err := l.releases.Get(ctx, id)
	return toRef(r), err
}

func (l Lookup) ResolveRelease(ctx context.Context, serviceSlug, ref string) (entities.ReleaseRef, error) {
	r, err := l.releases.Resolve(ctx, serviceSlug, ref)
	return toRef(r), err
}

func toRef(r release.Release) entities.ReleaseRef {
	return entities.ReleaseRef{
		ID: r.ID, ServiceID: r.ServiceID, Number: r.Number,
		Built: r.Status == release.StatusBuilt && r.Digest != "", Reference: r.Reference(),
	}
}

package configs

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/approval"
)

func (u ConfigUsecase) Patch(ctx context.Context, a actor.Actor, serviceSlug, environment string, patch []byte) (entities.Revision, error) {
	e, err := u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	cs, err := entities.PatchChangeSet(e.current, patch)
	if err != nil {
		return entities.Revision{}, err
	}
	return u.change(ctx, a, e, approval.KindConfigRevise, cs)
}

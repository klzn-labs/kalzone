package onboarding

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/onboarding/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errServiceNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")

type Services interface {
	FindService(ctx context.Context, slug string) (entities.Service, error)
}

type GitHub interface {
	Installed(ctx context.Context, repository string) (bool, error)
	DefaultBranch(ctx context.Context, repository string) (string, error)
	File(ctx context.Context, repository, path, ref string) (string, bool, error)
	OpenPullRequest(ctx context.Context, repository, base string, pr entities.PullRequest) (entities.OpenedPullRequest, error)
}

type Registry interface {
	CanPush(ctx context.Context, imageRepository string) (bool, error)
}

type Configs interface {
	DevSpec(ctx context.Context, serviceSlug string) (entities.DevSpec, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type OnboardingUsecase struct {
	services      Services
	github        GitHub
	registry      Registry
	configs       Configs
	authz         Authorizer
	buildWorkflow string
}

func NewOnboardingUsecase(services Services, github GitHub, registry Registry, configs Configs, authz Authorizer, buildWorkflow string) OnboardingUsecase {
	return OnboardingUsecase{services: services, github: github, registry: registry, configs: configs, authz: authz, buildWorkflow: buildWorkflow}
}

func (u OnboardingUsecase) visible(ctx context.Context, a actor.Actor, slug string) (entities.Service, error) {
	service, err := u.services.FindService(ctx, slug)
	if err == nil && !a.Sees(service.OwnerTeamID) {
		return entities.Service{}, errServiceNotVisible
	}
	return service, err
}

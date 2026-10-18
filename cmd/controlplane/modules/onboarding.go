package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	onboardingHandler "github.com/klzn-labs/kalzone/app/onboarding/handlers/onboarding"
	onboardingCatalog "github.com/klzn-labs/kalzone/app/onboarding/repositories/catalog"
	onboardingConfig "github.com/klzn-labs/kalzone/app/onboarding/repositories/envconfig"
	onboardingGitHub "github.com/klzn-labs/kalzone/app/onboarding/repositories/github"
	"github.com/klzn-labs/kalzone/app/onboarding/repositories/registry"
	"github.com/klzn-labs/kalzone/app/onboarding/usecases/onboarding"
	"github.com/klzn-labs/kalzone/app/release/repositories/githubapp"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

func Onboarding() fx.Option {
	return fx.Options(
		fx.Provide(
			onboardingHandler.NewOnboardingHandler,
			func(u services.ServicesUsecase) onboardingCatalog.ServiceFinder { return u },
			onboardingCatalog.NewLookup,
			func(u configs.ConfigUsecase) onboardingConfig.Configs { return u },
			onboardingConfig.NewSource,
			func(c githubapp.Client) onboardingGitHub.App { return c },
			onboardingGitHub.NewRepositories,
			newOnboardingUsecase,
			func(u onboarding.OnboardingUsecase) onboardingHandler.Onboarding { return u },
		),
		fx.Invoke(onboardingHandler.RegisterOnboardingRoutes),
	)
}

func newOnboardingUsecase(l onboardingCatalog.Lookup, g onboardingGitHub.Repositories, s onboardingConfig.Source, p authz.Policy, cfg config.Config) onboarding.OnboardingUsecase {
	r := registry.New(cfg.Registry.AuthURL, cfg.Registry.Username, cfg.Registry.Token)
	return onboarding.NewOnboardingUsecase(l, g, r, s, p, cfg.Build.Workflow)
}

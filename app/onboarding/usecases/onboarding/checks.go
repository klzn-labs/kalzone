package onboarding

import (
	"context"
	"strings"

	"github.com/klzn-labs/kalzone/app/onboarding/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const (
	hintApp        = "install the GitHub App on the repository, or set its installation to All repositories"
	hintRepository = "set github_repo on the service with updateService"
	hintWorkflow   = "call createOnboardingPullRequest, or add the file by hand"
	hintDockerfile = "add a Dockerfile at the repository root"
	hintRegistry   = "create the repository on Docker Hub under the platform's account, or set image_repository"
	hintConfig     = "write a dev config with port and health_path, for example with patchConfig"
	dependsOnApp   = "needs the github_app check"
)

func (u OnboardingUsecase) Check(ctx context.Context, a actor.Actor, slug string) (entities.Report, error) {
	service, err := u.visible(ctx, a, slug)
	if err != nil {
		return entities.Report{}, err
	}
	app := u.appCheck(ctx, service)
	checks := []entities.Check{app}
	if app.Status == entities.StatusOK {
		checks = append(checks, u.workflowCheck(ctx, service), u.dockerfileCheck(ctx, service))
	} else {
		checks = append(checks, skipped(entities.CheckWorkflow), skipped(entities.CheckDockerfile))
	}
	checks = append(checks, u.registryCheck(ctx, service), u.configCheck(ctx, service))
	return entities.NewReport(service.Slug, checks), nil
}

func skipped(name string) entities.Check {
	return entities.Check{Name: name, Status: entities.StatusSkipped, Detail: dependsOnApp}
}

func ok(name, detail string) entities.Check {
	return entities.Check{Name: name, Status: entities.StatusOK, Detail: detail}
}

func failed(name, detail, hint string) entities.Check {
	return entities.Check{Name: name, Status: entities.StatusFailed, Detail: detail, Hint: hint}
}

func (u OnboardingUsecase) appCheck(ctx context.Context, s entities.Service) entities.Check {
	if s.Repository == "" {
		return failed(entities.CheckGitHubApp, "the service has no GitHub repository", hintRepository)
	}
	installed, err := u.github.Installed(ctx, s.Repository)
	if err != nil {
		return failed(entities.CheckGitHubApp, err.Error(), hintApp)
	}
	if !installed {
		return failed(entities.CheckGitHubApp, "the GitHub App cannot see "+s.Repository, hintApp)
	}
	return ok(entities.CheckGitHubApp, "the GitHub App is installed on "+s.Repository)
}

func (u OnboardingUsecase) workflowCheck(ctx context.Context, s entities.Service) entities.Check {
	content, found, err := u.defaultBranchFile(ctx, s, s.WorkflowPath())
	switch {
	case err != nil:
		return failed(entities.CheckWorkflow, err.Error(), hintWorkflow)
	case !found:
		return failed(entities.CheckWorkflow, s.WorkflowPath()+" is missing on the default branch", hintWorkflow)
	case !strings.Contains(content, u.buildWorkflow+"@"):
		return failed(entities.CheckWorkflow, s.WorkflowPath()+" does not call "+u.buildWorkflow, hintWorkflow)
	}
	return ok(entities.CheckWorkflow, s.WorkflowPath()+" calls "+u.buildWorkflow)
}

func (u OnboardingUsecase) dockerfileCheck(ctx context.Context, s entities.Service) entities.Check {
	_, found, err := u.defaultBranchFile(ctx, s, "Dockerfile")
	if err != nil || !found {
		return failed(entities.CheckDockerfile, "no Dockerfile at the repository root", hintDockerfile)
	}
	return ok(entities.CheckDockerfile, "Dockerfile found")
}

func (u OnboardingUsecase) defaultBranchFile(ctx context.Context, s entities.Service, path string) (string, bool, error) {
	branch, err := u.github.DefaultBranch(ctx, s.Repository)
	if err != nil {
		return "", false, err
	}
	return u.github.File(ctx, s.Repository, path, branch)
}

func (u OnboardingUsecase) registryCheck(ctx context.Context, s entities.Service) entities.Check {
	push, err := u.registry.CanPush(ctx, s.ImageRepository)
	if err != nil {
		return failed(entities.CheckRegistry, err.Error(), hintRegistry)
	}
	if !push {
		return failed(entities.CheckRegistry, "the platform's registry credentials cannot push to "+s.ImageRepository, hintRegistry)
	}
	return ok(entities.CheckRegistry, "the platform can push to "+s.ImageRepository)
}

func (u OnboardingUsecase) configCheck(ctx context.Context, s entities.Service) entities.Check {
	spec, err := u.configs.DevSpec(ctx, s.Slug)
	if err != nil {
		return failed(entities.CheckConfig, err.Error(), hintConfig)
	}
	if !spec.Found || spec.Port == 0 || spec.HealthPath == "" {
		return failed(entities.CheckConfig, "dev needs a config with port and health_path", hintConfig)
	}
	return ok(entities.CheckConfig, "dev listens on "+itoa(spec.Port)+" with health_path "+spec.HealthPath)
}

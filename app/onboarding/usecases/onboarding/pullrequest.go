package onboarding

import (
	"context"
	"strconv"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/onboarding/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const pullRequestBody = "Opened by the platform's onboarding: builds of this repository run through the platform's reusable workflow, " +
	"which receives registry credentials from the platform, so the repository needs no Docker secrets."

var (
	errWorkflowPresent = apperror.Conflict("the repository already has the build workflow").WithHint("call getOnboarding to see what is left")
	errGitHubRefused   = apperror.New(apperror.CodeUpstream, "GitHub refused to open the pull request").
				WithHint("grant the GitHub App Contents, Pull requests and Workflows write, and accept them on the installation")
)

func (u OnboardingUsecase) OpenPullRequest(ctx context.Context, a actor.Actor, slug string) (entities.OpenedPullRequest, error) {
	service, err := u.visible(ctx, a, slug)
	if err != nil {
		return entities.OpenedPullRequest{}, err
	}
	if err := u.authz.Can(ctx, a, authz.EditService, authz.Resource{TeamID: service.OwnerTeamID}); err != nil {
		return entities.OpenedPullRequest{}, err
	}
	if u.workflowCheck(ctx, service).Status == entities.StatusOK {
		return entities.OpenedPullRequest{}, errWorkflowPresent
	}
	base, err := u.github.DefaultBranch(ctx, service.Repository)
	if err != nil {
		return entities.OpenedPullRequest{}, errGitHubRefused.WithDetails(map[string]any{"cause": err.Error()})
	}
	opened, err := u.github.OpenPullRequest(ctx, service.Repository, base, u.pullRequest(service))
	if err != nil {
		return entities.OpenedPullRequest{}, errGitHubRefused.WithDetails(map[string]any{"cause": err.Error()})
	}
	return opened, nil
}

func (u OnboardingUsecase) pullRequest(s entities.Service) entities.PullRequest {
	return entities.PullRequest{
		Branch: entities.OnboardingBranch, Path: s.WorkflowPath(), Content: entities.Workflow(u.buildWorkflow),
		Message: "Build with the platform", Title: "Build with the platform", Body: pullRequestBody,
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

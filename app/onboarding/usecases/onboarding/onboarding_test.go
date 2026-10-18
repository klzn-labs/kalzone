package onboarding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/onboarding/entities"
	"github.com/klzn-labs/kalzone/app/onboarding/usecases/onboarding"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

const buildWorkflow = "klzn-labs/kalzone-actions/.github/workflows/build.yml"

var (
	ctx      = context.Background()
	ana      = actor.Actor{UserID: "ana", Login: "ana"}
	outsider = actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}
	hello    = entities.Service{ID: "s1", Slug: "hello", OwnerTeamID: "t1", Repository: "o/hello", ImageRepository: "docker.io/o/hello"}
)

type services struct{ service entities.Service }

func (s services) FindService(_ context.Context, slug string) (entities.Service, error) {
	if slug != s.service.Slug {
		return entities.Service{}, apperror.NotFound("service not found")
	}
	return s.service, nil
}

type github struct {
	installed bool
	files     map[string]string
	opened    []entities.PullRequest
	err       error
}

func (g *github) Installed(context.Context, string) (bool, error) { return g.installed, nil }

func (g *github) DefaultBranch(context.Context, string) (string, error) { return "main", nil }

func (g *github) File(_ context.Context, _, path, ref string) (string, bool, error) {
	content, ok := g.files[ref+":"+path]
	return content, ok, nil
}

func (g *github) OpenPullRequest(_ context.Context, _, base string, pr entities.PullRequest) (entities.OpenedPullRequest, error) {
	if g.err != nil {
		return entities.OpenedPullRequest{}, g.err
	}
	g.opened = append(g.opened, pr)
	return entities.OpenedPullRequest{URL: "https://github.com/o/hello/pull/1", Number: 1, Branch: pr.Branch}, nil
}

type registry struct{ push bool }

func (r registry) CanPush(context.Context, string) (bool, error) { return r.push, nil }

type configs struct{ spec entities.DevSpec }

func (c configs) DevSpec(context.Context, string) (entities.DevSpec, error) { return c.spec, nil }

type policy struct{ err error }

func (p policy) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error { return p.err }

func healthy() *github {
	return &github{installed: true, files: map[string]string{
		"main:.github/workflows/platform-build.yml": entities.Workflow(buildWorkflow),
		"main:Dockerfile": "FROM scratch",
	}}
}

func usecase(g *github, push bool, spec entities.DevSpec, p policy) onboarding.OnboardingUsecase {
	return onboarding.NewOnboardingUsecase(services{hello}, g, registry{push}, configs{spec}, p, buildWorkflow)
}

var devSpec = entities.DevSpec{Found: true, Port: 8080, HealthPath: "/healthz"}

func statuses(r entities.Report) map[string]string {
	out := map[string]string{}
	for _, c := range r.Checks {
		out[c.Name] = c.Status
	}
	return out
}

func Test_Check_PassesWhenEverythingIsInPlace(t *testing.T) {
	report, err := usecase(healthy(), true, devSpec, policy{}).Check(ctx, ana, "hello")

	require.NoError(t, err)
	require.True(t, report.Ready)
	require.Len(t, report.Checks, 5)
}

func Test_Check_SkipsTheRepositoryChecksWithoutTheApp(t *testing.T) {
	g := healthy()
	g.installed = false

	report, _ := usecase(g, true, devSpec, policy{}).Check(ctx, ana, "hello")

	require.False(t, report.Ready)
	require.Equal(t, map[string]string{"github_app": "failed", "workflow": "skipped", "dockerfile": "skipped", "registry": "ok", "config": "ok"}, statuses(report))
	require.Contains(t, report.Checks[0].Hint, "All repositories")
}

func Test_Check_FailsAWorkflowThatDoesNotCallThePlatform(t *testing.T) {
	g := healthy()
	g.files["main:.github/workflows/platform-build.yml"] = "jobs: {}"
	delete(g.files, "main:Dockerfile")

	report, _ := usecase(g, false, entities.DevSpec{Found: true, Port: 8080}, policy{}).Check(ctx, ana, "hello")

	require.Equal(t, map[string]string{"github_app": "ok", "workflow": "failed", "dockerfile": "failed", "registry": "failed", "config": "failed"}, statuses(report))
	require.Contains(t, report.Checks[1].Hint, "createOnboardingPullRequest")
}

func Test_Check_HidesAServiceOfAnotherTeam(t *testing.T) {
	_, err := usecase(healthy(), true, devSpec, policy{}).Check(ctx, outsider, "hello")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_OpenPullRequest_AddsTheWorkflowOnABranch(t *testing.T) {
	g := healthy()
	delete(g.files, "main:.github/workflows/platform-build.yml")

	opened, err := usecase(g, true, devSpec, policy{}).OpenPullRequest(ctx, ana, "hello")

	require.NoError(t, err)
	require.Equal(t, 1, opened.Number)
	require.Equal(t, "platform-onboarding", g.opened[0].Branch)
	require.Equal(t, ".github/workflows/platform-build.yml", g.opened[0].Path)
	require.Equal(t, entities.Workflow(buildWorkflow), g.opened[0].Content)
}

func Test_OpenPullRequest_RefusesWhenTheWorkflowIsThereOrNotAllowed(t *testing.T) {
	missing := healthy()
	delete(missing.files, "main:.github/workflows/platform-build.yml")
	upstream := healthy()
	delete(upstream.files, "main:.github/workflows/platform-build.yml")
	upstream.err = errors.New("github create branch: 403")

	_, present := usecase(healthy(), true, devSpec, policy{}).OpenPullRequest(ctx, ana, "hello")
	_, forbidden := usecase(missing, true, devSpec, policy{err: apperror.Forbidden("no")}).OpenPullRequest(ctx, ana, "hello")
	_, refused := usecase(upstream, true, devSpec, policy{}).OpenPullRequest(ctx, ana, "hello")

	require.ErrorIs(t, present, apperror.Conflict(""))
	require.ErrorIs(t, forbidden, apperror.Forbidden(""))
	require.Equal(t, apperror.CodeUpstream, apperror.CodeOf(refused))
	var appErr *apperror.AppError
	require.ErrorAs(t, refused, &appErr)
	require.Contains(t, appErr.Hint, "Workflows")
}

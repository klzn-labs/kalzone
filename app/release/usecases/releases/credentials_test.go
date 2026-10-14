package releases_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/app/release/usecases/releases"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/stretchr/testify/require"
)

const buildWorkflow = "adrianozp/platform/.github/workflows/build.yml"

var buildCaller = entities.BuildCaller{Repository: "o/api", WorkflowRef: buildWorkflow + "@refs/heads/main"}

func withRegistry(f fixture, username, token string) releases.ReleasesUsecase {
	cfg := config.Config{
		BaseURL: "https://platform.test", Build: config.Build{Timeout: time.Hour, Workflow: buildWorkflow},
		Registry: config.Registry{Username: username, Token: token},
	}
	return releases.NewReleasesUsecase(cfg, f.store, services{}, f.builder, resolver{}, allow{false}, f.events)
}

func Test_RegistryCredentials_GoToTheServicesOwnBuildWhileItRuns(t *testing.T) {
	f := newFixture(t, false)
	u := withRegistry(f, "bot", "dckr_pat_x")
	rel, _ := u.Request(ctx, ana, "api", "main")

	creds, err := u.RegistryCredentials(ctx, buildCaller, rel.ID)

	require.NoError(t, err)
	require.Equal(t, entities.RegistryCredentials{Registry: "docker.io", Username: "bot", Password: "dckr_pat_x"}, creds)
	require.Contains(t, f.events.actions, "registry.credentials_issued")
}

func Test_RegistryCredentials_RefuseOtherRepositoriesWorkflowsAndFinishedBuilds(t *testing.T) {
	f := newFixture(t, false)
	u := withRegistry(f, "bot", "dckr_pat_x")
	rel, _ := u.Request(ctx, ana, "api", "main")
	other := entities.BuildCaller{Repository: "evil/repo", WorkflowRef: buildCaller.WorkflowRef}
	forked := entities.BuildCaller{Repository: "o/api", WorkflowRef: "o/api/.github/workflows/steal.yml@refs/heads/main"}
	lookalike := entities.BuildCaller{Repository: "o/api", WorkflowRef: buildWorkflow + "-evil.yml@refs/heads/main"}

	_, wrongRepo := u.RegistryCredentials(ctx, other, rel.ID)
	_, wrongWorkflow := u.RegistryCredentials(ctx, forked, rel.ID)
	_, prefix := u.RegistryCredentials(ctx, lookalike, rel.ID)
	_, err := u.Failed(ctx, buildCaller, rel.ID, entities.BuildReport{Failure: "boom"})
	require.NoError(t, err)
	_, finished := u.RegistryCredentials(ctx, buildCaller, rel.ID)

	for _, refused := range []error{wrongRepo, wrongWorkflow, prefix, finished} {
		require.ErrorIs(t, refused, apperror.Forbidden(""))
	}
	require.NotContains(t, f.events.actions, "registry.credentials_issued")
}

func Test_RegistryCredentials_NeedTheRegistryConfigured(t *testing.T) {
	f := newFixture(t, false)
	u := withRegistry(f, "", "")
	rel, _ := u.Request(ctx, ana, "api", "main")

	_, err := u.RegistryCredentials(ctx, buildCaller, rel.ID)

	require.Equal(t, apperror.CodeNotConfigured, apperror.CodeOf(err))
}

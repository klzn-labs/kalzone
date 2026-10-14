package releases

import (
	"context"
	"errors"
	"strings"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/apperror"
)

const dockerHub = "docker.io"

var (
	errNotTheBuildWorkflow = apperror.Forbidden("only the platform's build workflow gets registry credentials").
				WithHint("call adrianozp/platform/.github/workflows/build.yml from the repository's platform-build.yml")
	errBuildFinished = apperror.Forbidden("this release is not being built").WithHint("request a new release")
	errNoRegistry    = apperror.New(apperror.CodeNotConfigured, "the platform holds no registry credentials").
				WithHint("set PLATFORM_REGISTRY_USERNAME and PLATFORM_REGISTRY_TOKEN, or log in to the registry in the workflow")
)

func (u ReleasesUsecase) RegistryCredentials(ctx context.Context, caller entities.BuildCaller, releaseID string) (entities.RegistryCredentials, error) {
	if !strings.HasPrefix(caller.WorkflowRef, u.buildWorkflow+"@") {
		return entities.RegistryCredentials{}, errNotTheBuildWorkflow
	}
	p, err := u.pendingFor(ctx, caller, releaseID)
	if errors.Is(err, errAlreadyFinal) {
		return entities.RegistryCredentials{}, errBuildFinished
	}
	if err != nil {
		return entities.RegistryCredentials{}, err
	}
	if u.registry.Username == "" || u.registry.Token == "" {
		return entities.RegistryCredentials{}, errNoRegistry
	}
	u.publish(ctx, GitHubActor, "registry.credentials_issued", p.service, p.release)
	return entities.RegistryCredentials{Registry: dockerHub, Username: u.registry.Username, Password: u.registry.Token}, nil
}

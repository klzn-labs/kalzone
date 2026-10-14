package github

import (
	"context"

	"github.com/adrianozp/platform/app/onboarding/entities"
	"github.com/adrianozp/platform/app/release/repositories/githubapp"
)

type App interface {
	Installed(ctx context.Context, repository string) (bool, error)
	DefaultBranch(ctx context.Context, repository string) (string, error)
	File(ctx context.Context, repository, path, ref string) (string, bool, error)
	OpenPullRequest(ctx context.Context, repository, base string, pr githubapp.PullRequest) (githubapp.OpenedPullRequest, error)
}

type Repositories struct {
	app App
}

func NewRepositories(app App) Repositories {
	return Repositories{app: app}
}

func (r Repositories) Installed(ctx context.Context, repository string) (bool, error) {
	return r.app.Installed(ctx, repository)
}

func (r Repositories) DefaultBranch(ctx context.Context, repository string) (string, error) {
	return r.app.DefaultBranch(ctx, repository)
}

func (r Repositories) File(ctx context.Context, repository, path, ref string) (string, bool, error) {
	return r.app.File(ctx, repository, path, ref)
}

func (r Repositories) OpenPullRequest(ctx context.Context, repository, base string, pr entities.PullRequest) (entities.OpenedPullRequest, error) {
	opened, err := r.app.OpenPullRequest(ctx, repository, base, githubapp.PullRequest(pr))
	return entities.OpenedPullRequest(opened), err
}

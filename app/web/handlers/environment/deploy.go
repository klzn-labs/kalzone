package environment

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Deployments interface {
	History(ctx context.Context, serviceSlug, environment string, limit int) ([]deploy.Deployment, error)
	Live(ctx context.Context, serviceSlug, environment string) (deploy.Deployment, bool, error)
	Get(ctx context.Context, id string) (deploy.Deployment, []deploy.Event, error)
	Deploy(ctx context.Context, a actor.Actor, r deployments.Request) (deploy.Deployment, error)
	ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (deploy.Deployment, error)
	Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (deploy.Deployment, error)
	Rollback(ctx context.Context, a actor.Actor, deploymentID string) (deploy.Deployment, error)
}

type Releases interface {
	List(ctx context.Context, serviceSlug string, limit int) ([]release.Release, error)
}

type DeploymentRow struct {
	Deployment  deploy.Deployment
	RequestedBy identity.User
	IsLive      bool
}

type DeployView struct {
	Live         deploy.Deployment
	HasLive      bool
	Current      *deploy.Deployment
	Log          []deploy.Event
	Rows         []DeploymentRow
	Releases     []release.Release
	CanDeploy    bool
	ConfigBehind bool
	PromoteFrom  string
	PromoteLive  deploy.Deployment
}

func (h EnvironmentHandler) deployView(ctx context.Context, a actor.Actor, d data) (DeployView, error) {
	v, err := h.liveAndHistory(ctx, d.Service.Slug, d.Env.Slug)
	if err != nil {
		return DeployView{}, err
	}
	if v.Releases, err = h.builtReleases(ctx, d.Service.Slug); err != nil {
		return DeployView{}, err
	}
	v.CanDeploy = h.views.Can(ctx, a, authz.Deploy, authz.Resource{TeamID: d.Service.OwnerTeamID, Environment: d.Env.Slug})
	v.ConfigBehind = v.HasLive && v.Live.ConfigRevision != d.Revision.Number
	if d.Env.Protected {
		v.PromoteFrom, v.PromoteLive, err = h.promotionSource(ctx, d.Service.Slug)
	}
	return v, err
}

func (h EnvironmentHandler) liveAndHistory(ctx context.Context, serviceSlug, envSlug string) (DeployView, error) {
	var v DeployView
	var err error
	if v.Live, v.HasLive, err = h.deployments.Live(ctx, serviceSlug, envSlug); err != nil {
		return DeployView{}, err
	}
	history, err := h.deployments.History(ctx, serviceSlug, envSlug, 15)
	if err != nil {
		return DeployView{}, err
	}
	v.Rows, err = h.deploymentRows(ctx, history, v.Live.ID)
	v.Current = active(history)
	if err == nil && v.Current != nil {
		_, v.Log, err = h.deployments.Get(ctx, v.Current.ID)
	}
	return v, err
}

func (h EnvironmentHandler) deploymentRows(ctx context.Context, history []deploy.Deployment, liveID string) ([]DeploymentRow, error) {
	ids := make([]string, 0, len(history))
	for _, d := range history {
		ids = append(ids, d.RequestedBy)
	}
	users, err := h.views.UsersByID(ctx, ids)
	rows := make([]DeploymentRow, 0, len(history))
	for _, d := range history {
		rows = append(rows, DeploymentRow{Deployment: d, RequestedBy: users[d.RequestedBy], IsLive: d.ID == liveID})
	}
	return rows, err
}

func active(history []deploy.Deployment) *deploy.Deployment {
	for i := range history {
		if history[i].Active() {
			return &history[i]
		}
	}
	return nil
}

func (h EnvironmentHandler) builtReleases(ctx context.Context, serviceSlug string) ([]release.Release, error) {
	all, err := h.releases.List(ctx, serviceSlug, 30)
	built := make([]release.Release, 0, len(all))
	for _, r := range all {
		if r.Status == release.StatusBuilt {
			built = append(built, r)
		}
	}
	return built, err
}

func (h EnvironmentHandler) promotionSource(ctx context.Context, serviceSlug string) (string, deploy.Deployment, error) {
	envs, err := h.views.Environments(ctx)
	if err != nil {
		return "", deploy.Deployment{}, err
	}
	for _, env := range envs {
		if env.Protected {
			continue
		}
		live, found, err := h.deployments.Live(ctx, serviceSlug, env.Slug)
		if err != nil || found {
			return env.Slug, live, err
		}
	}
	return "", deploy.Deployment{}, nil
}

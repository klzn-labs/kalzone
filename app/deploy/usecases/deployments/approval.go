package deployments

import (
	"context"
	"fmt"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

var errNotApproved = apperror.Forbidden("only an approved request can apply this change")

type Proposal struct {
	Kind         string          `json:"kind"`
	Service      string          `json:"service"`
	Environment  string          `json:"environment"`
	ReleaseID    string          `json:"release_id,omitempty"`
	Strategy     string          `json:"strategy,omitempty"`
	From         string          `json:"from,omitempty"`
	DeploymentID string          `json:"deployment_id,omitempty"`
	LiveID       string          `json:"live_id,omitempty"`
	Ships        *approval.Ships `json:"ships,omitempty"`
}

func (u DeploymentsUsecase) propose(ctx context.Context, p plan, o Proposal) error {
	live, _, err := u.store.LatestSucceeded(ctx, p.service.ID, p.env)
	if err != nil {
		return err
	}
	o.Service, o.Environment, o.LiveID = p.service.Slug, p.env, live.ID
	o.Ships = u.shipsFor(ctx, o.Kind, live.ReleaseID, p.release.ID)
	pending, err := u.requests.Submit(ctx, p.actor, approval.Change{
		Kind: o.Kind, Subject: "deploy", ServiceID: p.service.ID, ServiceSlug: p.service.Slug,
		TeamID: p.service.OwnerTeamID, Environment: p.env, Summary: summaryOf(o.Kind, p), Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func summaryOf(kind string, p plan) string {
	switch kind {
	case approval.KindPromote:
		return fmt.Sprintf("promote release #%d to %s", p.release.Number, p.env)
	case approval.KindRollback:
		return fmt.Sprintf("roll %s back to release #%d with config #%d", p.env, p.release.Number, p.config)
	case approval.KindConfigApply:
		return fmt.Sprintf("apply config #%d to %s", p.config, p.env)
	case approval.KindRemove:
		return fmt.Sprintf("remove %s from %s", p.service.Slug, p.env)
	}
	return fmt.Sprintf("deploy release #%d to %s", p.release.Number, p.env)
}

func (u DeploymentsUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error) {
	if err := u.approved(ctx, a, o); err != nil {
		return entities.Deployment{}, err
	}
	if err := u.stillLive(ctx, o); err != nil {
		return entities.Deployment{}, err
	}
	return u.replay(ctx, a, o)
}

func (u DeploymentsUsecase) approved(ctx context.Context, a actor.Actor, o Proposal) error {
	p, err := u.authorizeAny(ctx, a, o.Service, o.Environment)
	if err != nil {
		return err
	}
	if p.gate != authz.Approved && p.gate != authz.Open {
		return errNotApproved
	}
	return nil
}

func (u DeploymentsUsecase) replay(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error) {
	switch o.Kind {
	case approval.KindPromote:
		return u.promotePinned(ctx, a, o)
	case approval.KindRollback:
		return u.Rollback(ctx, a, o.DeploymentID)
	case approval.KindConfigApply:
		return u.ApplyConfig(ctx, a, o.Service, o.Environment)
	case approval.KindRemove:
		return u.Remove(ctx, a, o.Service, o.Environment)
	}
	return u.Deploy(ctx, a, Request{Service: o.Service, Environment: o.Environment, ReleaseID: o.ReleaseID, Strategy: o.Strategy})
}

func (u DeploymentsUsecase) stillLive(ctx context.Context, o Proposal) error {
	live, _, err := u.Live(ctx, o.Service, o.Environment)
	if err != nil || live.ID == o.LiveID {
		return err
	}
	return apperror.Stale(fmt.Sprintf("deployment #%d went live after the request", live.Number))
}

func (u DeploymentsUsecase) promotePinned(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error) {
	p, err := u.authorize(ctx, a, o.Service, o.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if p.release, err = u.deployable(ctx, p.service, o.ReleaseID); err != nil {
		return entities.Deployment{}, err
	}
	if p, err = u.promoted(ctx, p); err != nil {
		return entities.Deployment{}, err
	}
	return u.proceed(ctx, p, Proposal{Kind: approval.KindPromote, ReleaseID: p.release.ID, From: o.From})
}

func (u DeploymentsUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	live, _, err := u.Live(ctx, o.Service, o.Environment)
	if err != nil {
		return approval.Preview{Ships: o.Ships}, err
	}
	lines, err := u.previewLines(ctx, o, live)
	if err != nil {
		return approval.Preview{Ships: o.Ships}, err
	}
	preview := approval.Preview{Lines: lines, Ships: o.Ships}
	if live.ID != o.LiveID {
		preview.Conflict = fmt.Sprintf("approving will fail: deployment #%d went live after the request", live.Number)
	}
	return preview, nil
}

func (u DeploymentsUsecase) previewLines(ctx context.Context, o Proposal, live entities.Deployment) ([]approval.Line, error) {
	if o.Kind == approval.KindRemove {
		return []approval.Line{{Key: "workload", Before: liveNumber(live, live.ReleaseNumber), After: "removed"}}, nil
	}
	target, err := u.targetOf(ctx, o, live)
	return []approval.Line{
		{Key: "release", Before: liveNumber(live, live.ReleaseNumber), After: fmt.Sprintf("#%d", target.ReleaseNumber)},
		{Key: "config", Before: liveNumber(live, live.ConfigRevision), After: fmt.Sprintf("#%d", target.ConfigRevision)},
	}, err
}

func (u DeploymentsUsecase) targetOf(ctx context.Context, o Proposal, live entities.Deployment) (entities.Deployment, error) {
	if o.Kind == approval.KindRollback {
		return u.store.FindByID(ctx, o.DeploymentID)
	}
	config, err := u.configs.CurrentNumber(ctx, o.Service, o.Environment)
	if err != nil || o.Kind == approval.KindConfigApply {
		return entities.Deployment{ReleaseNumber: live.ReleaseNumber, ConfigRevision: config}, err
	}
	rel, err := u.releases.FindRelease(ctx, o.ReleaseID)
	return entities.Deployment{ReleaseNumber: rel.Number, ConfigRevision: config}, err
}

func liveNumber(live entities.Deployment, number int) string {
	if live.ID == "" {
		return ""
	}
	return fmt.Sprintf("#%d", number)
}

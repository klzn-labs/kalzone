package deployments_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

type gated struct {
	store     *fakestore.Store
	requests  *requests
	events    *published
	shipments *shipments
}

func newGated() gated {
	return gated{store: fakestore.New(), requests: &requests{}, events: &published{}, shipments: &shipments{}}
}

func (g gated) as(gate authz.Gate) deployments.DeploymentsUsecase {
	return deployments.NewDeploymentsUsecase(g.store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{gate: gate}, g.events, g.requests, g.shipments)
}

func (g gated) per(gates map[string]authz.Gate) deployments.DeploymentsUsecase {
	return deployments.NewDeploymentsUsecase(g.store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{gates: gates}, g.events, g.requests, g.shipments)
}

func (g gated) live(t *testing.T, env, releaseID string) entities.Deployment {
	t.Helper()
	d, err := g.as(authz.Open).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: env, ReleaseID: releaseID})
	require.NoError(t, err)
	succeed(g.store, d)
	return d
}

func (g gated) proposal(t *testing.T) deployments.Proposal {
	t.Helper()
	raw, err := json.Marshal(g.requests.changes[len(g.requests.changes)-1].Proposal)
	require.NoError(t, err)
	var p deployments.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

var approvedAna = actor.Actor{UserID: "ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "mia"}}

func Test_Deploy_NeedingApprovalSubmitsInsteadOfScheduling(t *testing.T) {
	g := newGated()
	live := g.live(t, "prod", "r1")

	_, err := g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Len(t, g.store.ByID, 1)
	c := g.requests.changes[0]
	require.Equal(t, "deploy", c.Subject)
	require.Equal(t, "deploy release #2 to prod", c.Summary)
	require.Equal(t, deployments.Proposal{
		Kind: approval.KindDeploy, Service: "api", Environment: "prod", ReleaseID: "r2", Strategy: "bluegreen", LiveID: live.ID,
		Ships: &approval.Ships{State: "ahead", Direction: "forward", To: 2, CommitCount: 3},
	}, g.proposal(t))
}

func Test_Promote_PinsTheReleaseThatWasLiveWhenAsked(t *testing.T) {
	g := newGated()
	g.live(t, "dev", "r1")
	_, _ = g.as(authz.NeedsApproval).Promote(ctx, ana, "api", "dev", "prod")
	g.live(t, "dev", "r2")

	d, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 1, d.ReleaseNumber)
	require.Equal(t, entities.TriggerPromote, d.Trigger)
	require.Equal(t, "promote release #1 to prod", g.requests.changes[0].Summary)
}

func Test_ApplyApproved_FailsWhenAnotherDeploymentWentLive(t *testing.T) {
	g := newGated()
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})
	g.live(t, "prod", "r1")

	_, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.Equal(t, apperror.CodeStale, apperror.CodeOf(err))
}

func Test_ApplyApproved_RollsBackToThePinnedDeployment(t *testing.T) {
	g := newGated()
	first := g.live(t, "prod", "r1")
	g.live(t, "prod", "r2")
	_, _ = g.as(authz.NeedsApproval).Rollback(ctx, ana, first.ID)

	d, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 1, d.ReleaseNumber)
	require.Equal(t, entities.TriggerRollback, d.Trigger)
}

func Test_ApplyApproved_RefusesAnActorWithoutAnApprovalAndProposesNothing(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})
	p := g.proposal(t)

	_, needsApproval := g.as(authz.NeedsApproval).ApplyApproved(ctx, ana, p)
	_, bypassed := g.as(authz.Bypassed).ApplyApproved(ctx, ana, p)

	require.ErrorIs(t, needsApproval, apperror.Forbidden(""))
	require.ErrorIs(t, bypassed, apperror.Forbidden(""))
	require.Len(t, g.requests.changes, 1)
	require.Len(t, g.store.ByID, 1)
}

func Test_ApplyApproved_RunsForASignedInHuman(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	d, err := g.as(authz.Open).ApplyApproved(ctx, ana, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 2, d.ReleaseNumber)
}

func Test_Schedule_PublishesTheBypassWithTheDeployment(t *testing.T) {
	g := newGated()

	_, err := g.as(authz.Bypassed).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r1"})

	require.NoError(t, err)
	require.Equal(t, []string{"deploy.requested", "token.bypass_used"}, g.events.actions)
}

func Test_Preview_ShowsWhatIsLiveAgainstTheTarget(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})
	p := g.proposal(t)
	g.live(t, "prod", "r1")

	preview, err := g.as(authz.Open).Preview(ctx, p)

	require.NoError(t, err)
	require.Equal(t, approval.Line{Key: "release", Before: "#1", After: "#2"}, preview.Lines[0])
	require.Equal(t, approval.Line{Key: "config", Before: "#2", After: "#2"}, preview.Lines[1])
	require.Contains(t, preview.Conflict, "deployment #2 went live")
}

package configs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/stretchr/testify/require"
)

var approvedAna = actor.Actor{UserID: "ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverID: "mia", ApproverLogin: "mia"}}

func setEnv(key, value string) entities.ChangeSet {
	return entities.ChangeSet{SetEnv: map[string]string{key: value}}
}

func proposalOf(t *testing.T, c approval.Change) configs.Proposal {
	t.Helper()
	raw, err := json.Marshal(c.Proposal)
	require.NoError(t, err)
	var p configs.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

func Test_Revise_NeedingApprovalSubmitsTheChangeInsteadOfCommitting(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, err := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Empty(t, f.store.revisions)
	c := f.requests.changes[0]
	require.Equal(t, approval.KindConfigRevise, c.Kind)
	require.Equal(t, "config", c.Subject)
	require.Equal(t, "change env.A", c.Summary)
	require.Equal(t, configs.Proposal{Kind: approval.KindConfigRevise, Service: "api", Environment: "prod", Change: setEnv("A", "1")}, proposalOf(t, c))
}

func Test_Revise_NeedingApprovalValidatesFirst(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, invalid := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("lower", "1"))
	_, noop := f.usecase.Revise(ctx, ana, "api", "prod", entities.ChangeSet{})

	require.ErrorIs(t, invalid, apperror.Invalid(""))
	require.ErrorIs(t, noop, apperror.Invalid(""))
	require.Empty(t, f.requests.changes)
}

func Test_SetSecret_NeedingApprovalSealsNowAndNamesOnlyTheKey(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, err := f.usecase.SetSecret(ctx, ana, "api", "prod", "TOKEN", "hunter2")

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Len(t, f.store.versions, 1)
	c := f.requests.changes[0]
	require.Equal(t, "secret.TOKEN", c.Subject)
	require.Equal(t, "set secret.TOKEN", c.Summary)
	require.Equal(t, map[string]string{"TOKEN": f.store.versions[0].ID}, proposalOf(t, c).Change.SetSecrets)
	require.NotContains(t, fmt.Sprint(c), "hunter2")
}

func requestThenMove(t *testing.T, f fixture, request, meanwhile entities.ChangeSet) configs.Proposal {
	t.Helper()
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.Revise(ctx, ana, "api", "prod", request)
	f.checks.gate = authz.Open
	_, err := f.usecase.Revise(ctx, ana, "api", "prod", meanwhile)
	require.NoError(t, err)
	f.checks.gate = authz.Approved
	return proposalOf(t, f.requests.changes[0])
}

func Test_ApplyApproved_ReappliesTheChangeOnTheCurrentRevision(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("B", "2"), setEnv("A", "9"))

	rev, err := f.usecase.ApplyApproved(ctx, approvedAna, p)

	require.NoError(t, err)
	require.Equal(t, 2, rev.Number)
	require.Equal(t, map[string]string{"A": "9", "B": "2"}, rev.Env)
	last := f.events.events[len(f.events.events)-1]
	require.Equal(t, "c1", last.Details["request_id"])
	require.Equal(t, "mia", last.Details["approved_by"])
}

func Test_ApplyApproved_FailsWhenTheSameKeyMoved(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("A", "2"), setEnv("A", "9"))

	_, err := f.usecase.ApplyApproved(ctx, approvedAna, p)

	require.Equal(t, apperror.CodeStale, apperror.CodeOf(err))
	require.Contains(t, err.Error(), "env.A")
	require.Len(t, f.store.revisions, 1)
}

func Test_Preview_DiffsAgainstTheCurrentRevisionAndWarns(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("A", "2"), setEnv("A", "9"))

	preview, err := f.usecase.Preview(ctx, p)

	require.NoError(t, err)
	require.Equal(t, []approval.Line{{Key: "env.A", Before: "9", After: "2"}}, preview.Lines)
	require.Contains(t, preview.Moved, "from revision 0 to 1")
	require.Contains(t, preview.Conflict, "env.A")
}

func Test_Preview_HidesSecretValues(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.SetSecret(ctx, ana, "api", "prod", "TOKEN", "hunter2")

	preview, err := f.usecase.Preview(ctx, proposalOf(t, f.requests.changes[0]))

	require.NoError(t, err)
	require.Equal(t, []approval.Line{{Key: "secret.TOKEN", After: "new value"}}, preview.Lines)
	require.Empty(t, preview.Moved)
}

func Test_Revise_PublishesTheBypassOnlyWhenTheChangeIsMade(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.Bypassed

	_, invalid := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("lower", "1"))
	_, err := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))

	require.Error(t, invalid)
	require.NoError(t, err)
	require.Len(t, f.events.events, 2)
	require.Equal(t, "config.revised", f.events.events[0].Action)
	require.Equal(t, "token.bypass_used", f.events.events[1].Action)
}

func Test_Revise_LeavesSecretsOutOfTheRequest(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval
	cs := setEnv("A", "1")
	cs.SetSecrets, cs.UnsetSecrets = map[string]string{"TOKEN": "v1"}, []string{"OLD"}

	_, _ = f.usecase.Revise(ctx, ana, "api", "prod", cs)

	change := proposalOf(t, f.requests.changes[0]).Change
	require.Empty(t, change.SetSecrets)
	require.Empty(t, change.UnsetSecrets)
	require.Equal(t, "change env.A", f.requests.changes[0].Summary)
}

func Test_ApplyApproved_ChecksTheActionOfTheKind(t *testing.T) {
	cases := map[string]authz.Action{
		approval.KindConfigRevise: authz.EditConfig,
		approval.KindSecretSet:    authz.EditSecrets,
		approval.KindSecretUnset:  authz.EditSecrets,
	}
	for kind, want := range cases {
		f := newFixture(t)
		f.checks.gate = authz.Approved
		_, _ = f.usecase.ApplyApproved(ctx, approvedAna, configs.Proposal{Kind: kind, Service: "api", Environment: "prod", Change: setEnv("A", "1")})
		require.Equal(t, []authz.Action{want}, f.checks.actions, kind)
	}
}

func Test_ApplyApproved_NeedsTheSecretsScopeForASecretChange(t *testing.T) {
	f := newFixtureAuthorizedBy(t, authz.NewPolicy(memberOfOwnerTeam{}, unprotected{}))
	configOnly := withToken(scope.Config)
	configOnly.Approval = approvedAna.Approval
	p := configs.Proposal{Kind: approval.KindSecretUnset, Service: "api", Environment: "dev", Change: entities.ChangeSet{UnsetSecrets: []string{"TOKEN"}}}

	_, err := f.usecase.ApplyApproved(ctx, configOnly, p)

	require.Equal(t, apperror.CodeMissingScope, apperror.CodeOf(err))
	require.Empty(t, f.store.revisions)
}

func Test_ApplyApproved_RefusesAnActorWithoutAnApprovalAndProposesNothing(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))
	p := proposalOf(t, f.requests.changes[0])

	_, needsApproval := f.usecase.ApplyApproved(ctx, ana, p)
	f.checks.gate = authz.Bypassed
	_, bypassed := f.usecase.ApplyApproved(ctx, ana, p)

	require.ErrorIs(t, needsApproval, apperror.Forbidden(""))
	require.ErrorIs(t, bypassed, apperror.Forbidden(""))
	require.Len(t, f.requests.changes, 1)
	require.Empty(t, f.store.revisions)
}

func Test_ApplyApproved_RunsForASignedInHuman(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))
	f.checks.gate = authz.Open

	rev, err := f.usecase.ApplyApproved(ctx, ana, proposalOf(t, f.requests.changes[0]))

	require.NoError(t, err)
	require.Equal(t, map[string]string{"A": "1"}, rev.Env)
}

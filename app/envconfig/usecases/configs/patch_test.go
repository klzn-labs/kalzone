package configs_test

import (
	"errors"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/stretchr/testify/require"
)

func Test_Patch_ChangesOnlyWhatThePatchNames(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"LOG_LEVEL": "info", "OLD_FLAG": "1"}})
	require.NoError(t, err)

	rev, err := f.usecase.Patch(ctx, ana, "api", "dev", []byte(`{"spec":{"replicas":2},"env":{"LOG_LEVEL":"debug","OLD_FLAG":null},"message":"more replicas"}`))

	require.NoError(t, err)
	require.Equal(t, 2, rev.Number)
	require.Equal(t, 2, rev.Spec.Replicas)
	require.Equal(t, entities.DefaultSpec().MemoryMB, rev.Spec.MemoryMB)
	require.Equal(t, map[string]string{"LOG_LEVEL": "debug"}, rev.Env)
	require.Equal(t, "more replicas", rev.Message)
}

func Test_Patch_GoesThroughTheSameChecksAsARevision(t *testing.T) {
	f := newFixture(t)
	f.checks.denyEnv = "prod"

	_, nothing := f.usecase.Patch(ctx, ana, "api", "dev", []byte(`{"spec":{"replicas":1}}`))
	_, invalid := f.usecase.Patch(ctx, ana, "api", "dev", []byte(`{"spec":{"replicas":0}}`))
	_, forbidden := f.usecase.Patch(ctx, ana, "api", "prod", []byte(`{"env":{"A":"1"}}`))

	require.Equal(t, "nothing changed", nothing.(*apperror.AppError).Message)
	require.ErrorIs(t, invalid, apperror.Invalid(""))
	require.ErrorIs(t, forbidden, apperror.Forbidden(""))
	require.Empty(t, f.store.revisions)
}

func Test_Patch_OnAProtectedEnvironmentAsksForApproval(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, err := f.usecase.Patch(ctx, ana, "api", "prod", []byte(`{"env":{"LOG_LEVEL":"debug"}}`))

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Equal(t, map[string]string{"LOG_LEVEL": "debug"}, proposalOf(t, f.requests.changes[0]).Change.SetEnv)
	require.Empty(t, f.store.revisions)
}

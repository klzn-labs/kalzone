package configs_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func Test_Current_StartsFromDefaults(t *testing.T) {
	f := newFixture(t)

	rev, err := f.usecase.Current(ctx, "api", "dev")

	require.NoError(t, err)
	require.Equal(t, 0, rev.Number)
	require.Equal(t, entities.DefaultSpec(), rev.Spec)
}

func Test_Current_UnknownServiceOrEnvironmentIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, svcErr := f.usecase.Current(ctx, "ghost", "dev")
	_, envErr := f.usecase.Current(ctx, "api", "staging")

	require.ErrorIs(t, svcErr, apperror.NotFound(""))
	require.ErrorIs(t, envErr, apperror.NotFound(""))
}

func Test_Revise_CopiesThePreviousRevisionAndAppliesTheChange(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"A": "1", "B": "2"}})
	require.NoError(t, err)

	second, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"A": "9"}, UnsetEnv: []string{"B"}, Message: " tweak "})

	require.NoError(t, err)
	require.Equal(t, 2, second.Number)
	require.Equal(t, map[string]string{"A": "9"}, second.Env)
	require.Equal(t, "tweak", second.Message)
	require.Equal(t, "ana", second.AuthorID)
	require.Equal(t, map[string]string{"A": "1", "B": "2"}, f.store.revisions[0].Env)
}

func Test_Revise_ChecksEditConfigForTheOwnerTeamAndEnvironment(t *testing.T) {
	f := newFixture(t)
	f.checks.denyEnv = "prod"

	_, devErr := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"A": "1"}})
	_, prodErr := f.usecase.Revise(ctx, ana, "api", "prod", entities.ChangeSet{SetEnv: map[string]string{"A": "1"}})

	require.NoError(t, devErr)
	require.ErrorIs(t, prodErr, apperror.Forbidden(""))
	require.Equal(t, authz.Resource{TeamID: "t1", Environment: "dev"}, f.checks.resources[0])
	require.Len(t, f.store.revisions, 1)
}

func Test_Revise_HostRules(t *testing.T) {
	f := newFixture(t)
	f.store.takenHost = "taken.kalz.one"
	withHost := func(host string) entities.ChangeSet {
		spec := entities.DefaultSpec()
		spec.Host = host
		return entities.ChangeSet{Spec: &spec}
	}

	_, foreign := f.usecase.Revise(ctx, ana, "api", "dev", withHost("api.example.com"))
	_, taken := f.usecase.Revise(ctx, ana, "api", "dev", withHost("taken.kalz.one"))
	_, free := f.usecase.Revise(ctx, ana, "api", "dev", withHost("api.kalz.one"))

	require.ErrorIs(t, foreign, apperror.Invalid(""))
	require.ErrorIs(t, taken, apperror.Conflict(""))
	require.NoError(t, free)
}

func Test_Revise_RejectsNoOpAndInvalidChanges(t *testing.T) {
	f := newFixture(t)

	_, noop := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{})
	_, invalid := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"lower": "x"}})

	require.ErrorIs(t, noop, apperror.Invalid(""))
	require.ErrorIs(t, invalid, apperror.Invalid(""))
	require.Empty(t, f.store.revisions)
}

func Test_Revise_PublishesChangedKeysWithoutValues(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"DB_HOST": "db.internal"}})

	require.NoError(t, err)
	event := f.events.events[0]
	require.Equal(t, "config.revised", event.Action)
	require.Equal(t, "dev", event.Environment)
	require.Equal(t, "t1", event.TeamID)
	require.Equal(t, []string{"env.DB_HOST"}, event.Details["changed"])
	require.NotContains(t, event.Details, "db.internal")
}

package configs_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/stretchr/testify/require"
)

type memberOfOwnerTeam struct{}

func (memberOfOwnerTeam) RoleOf(_ context.Context, teamID, _ string) (string, error) {
	if teamID != "t1" {
		return "", nil
	}
	return "member", nil
}

type unprotected struct{}

func (unprotected) IsProtected(context.Context, string) (bool, error) { return false, nil }

type discard struct{}

func (discard) Publish(context.Context, events.Event) {}

func withToken(scopes ...scope.Scope) actor.Actor {
	a := ana
	a.Token = actor.Token{ID: "tok", Name: "agent", Scopes: scopes, TeamIDs: []string{"t1"}}
	return a
}

func Test_Secrets_NeedTheSecretsScopeNotConfig(t *testing.T) {
	f := newFixtureAuthorizedBy(t, authz.NewPolicy(memberOfOwnerTeam{}, unprotected{}, discard{}))
	configOnly, secrets := withToken(scope.Config), withToken(scope.Secrets)

	_, setRefused := f.usecase.SetSecret(ctx, configOnly, "api", "dev", "TOKEN", "x")
	_, setErr := f.usecase.SetSecret(ctx, secrets, "api", "dev", "TOKEN", "x")
	_, unsetRefused := f.usecase.UnsetSecret(ctx, configOnly, "api", "dev", "TOKEN")
	_, unsetErr := f.usecase.UnsetSecret(ctx, secrets, "api", "dev", "TOKEN")

	require.Equal(t, apperror.CodeMissingScope, apperror.CodeOf(setRefused))
	require.Equal(t, apperror.CodeMissingScope, apperror.CodeOf(unsetRefused))
	require.NoError(t, setErr)
	require.NoError(t, unsetErr)
}

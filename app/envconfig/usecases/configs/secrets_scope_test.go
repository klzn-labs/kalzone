package configs_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

type memberOfOwnerTeam struct{}

func (memberOfOwnerTeam) RoleOf(_ context.Context, teamID, _ string) (string, error) {
	if teamID != "t1" {
		return "", nil
	}
	return "member", nil
}

func (memberOfOwnerTeam) IsDemo(context.Context, string) (bool, error) {
	return false, nil
}

type unprotected struct{}

func (unprotected) IsProtected(context.Context, string) (bool, error) { return false, nil }

func withToken(scopes ...scope.Scope) actor.Actor {
	a := ana
	a.Token = actor.Token{ID: "tok", Name: "agent", Scopes: scopes, TeamIDs: []string{"t1"}}
	return a
}

func Test_Secrets_NeedTheSecretsScopeNotConfig(t *testing.T) {
	f := newFixtureAuthorizedBy(t, authz.NewPolicy(memberOfOwnerTeam{}, unprotected{}))
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

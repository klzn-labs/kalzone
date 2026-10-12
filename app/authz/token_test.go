package authz_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/stretchr/testify/require"
)

func via(owner string, scopes []scope.Scope, teams []string, bypass bool) actor.Actor {
	a := people[owner]
	a.Token = actor.Token{ID: "tok", Name: "claude-local", Scopes: scopes, TeamIDs: teams, Bypass: bypass}
	return a
}

func only(s ...scope.Scope) []scope.Scope { return s }

var places = []string{"places"}

type tokenCase struct {
	name   string
	actor  actor.Actor
	action authz.Action
	env    string
	code   apperror.Code
}

var tokenCases = []tokenCase{
	{"a member's deploy token deploys dev", via("member", only(scope.Deploy), places, false), authz.Deploy, "dev", ""},
	{"a missing scope is named", via("member", only(scope.Read), places, false), authz.Deploy, "dev", apperror.CodeMissingScope},
	{"a team not on the token", via("member", only(scope.Deploy), []string{"reviews"}, false), authz.Deploy, "dev", apperror.CodeTeamNotGranted},
	{"the owner lacks the role", via("outsider", scope.All, places, true), authz.Deploy, "dev", apperror.CodeForbidden},
	{"a member owner never reaches prod", via("member", scope.All, places, true), authz.Deploy, "prod", apperror.CodeForbidden},
	{"prod without protected", via("maintainer", only(scope.Deploy), places, false), authz.Deploy, "prod", apperror.CodeMissingScope},
	{"prod without bypass", via("maintainer", only(scope.Deploy, scope.Protected), places, false), authz.Deploy, "prod", apperror.CodeApprovalRequired},
	{"prod with bypass", via("maintainer", only(scope.Deploy, scope.Protected), places, true), authz.Deploy, "prod", ""},
	{"config on prod needs approval too", via("maintainer", only(scope.Config, scope.Protected), places, false), authz.EditConfig, "prod", apperror.CodeApprovalRequired},
	{"secrets need their own scope", via("member", only(scope.Config), places, false), authz.EditSecrets, "dev", apperror.CodeMissingScope},
	{"secrets with the secrets scope", via("member", only(scope.Secrets), places, false), authz.EditSecrets, "dev", ""},
	{"queues go with services", via("member", only(scope.Services), places, false), authz.EditQueue, "dev", ""},
	{"deleting needs services:admin", via("maintainer", only(scope.Services), places, false), authz.DeleteService, "", apperror.CodeMissingScope},
	{"builds", via("member", only(scope.Builds), places, false), authz.CreateRelease, "", ""},
	{"an admin owner is bound by scopes", via("admin", only(scope.Read), places, false), authz.EditService, "", apperror.CodeMissingScope},
	{"an admin owner is bound by teams", via("admin", scope.All, []string{"reviews"}, true), authz.EditService, "", apperror.CodeTeamNotGranted},
	{"an admin owner's token manages teams with teams:admin", via("admin", only(scope.TeamsAdmin), places, false), authz.ManageTeams, "", ""},
	{"admins.manage is never for tokens", via("admin", scope.All, places, true), authz.ManageAdmins, "", apperror.CodeForbidden},
	{"tokens.manage needs tokens:admin", via("outsider", only(scope.Read), places, false), authz.ManageTokens, "", apperror.CodeMissingScope},
	{"tokens.manage with tokens:admin", via("outsider", only(scope.TokensAdmin), places, false), authz.ManageTokens, "", ""},
}

func Test_Policy_Tokens(t *testing.T) {
	for _, c := range tokenCases {
		t.Run(c.name, checkToken(c))
	}
}

func checkToken(c tokenCase) func(t *testing.T) {
	return func(t *testing.T) {
		err := policy.Can(context.Background(), c.actor, c.action, authz.Resource{TeamID: "places", Environment: c.env})
		requireCode(t, c.code, err)
	}
}

func requireCode(t *testing.T, code apperror.Code, err error) {
	t.Helper()
	if code == "" {
		require.NoError(t, err)
		return
	}
	require.Equal(t, code, apperror.CodeOf(err), "%v", err)
}

func Test_Policy_SessionsManageTheirOwnTokens(t *testing.T) {
	err := policy.Can(context.Background(), people["outsider"], authz.ManageTokens, authz.Resource{})

	require.NoError(t, err)
}

func Test_Policy_ErrorsSayWhatIsMissing(t *testing.T) {
	ctx := context.Background()
	scopeErr := policy.Can(ctx, via("member", only(scope.Read), places, false), authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"})
	teamErr := policy.Can(ctx, via("member", only(scope.Deploy), []string{"reviews"}, false), authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"})

	require.Equal(t, "deploy", asAppError(t, scopeErr).RequiredScope)
	require.Equal(t, "this token lacks deploy", scopeErr.Error())
	require.Equal(t, "places", asAppError(t, teamErr).Team)
}

func asAppError(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	appErr, ok := err.(*apperror.AppError)
	require.True(t, ok, "%T", err)
	return appErr
}

func Test_Policy_BypassIsAuditedOnlyWhenUsed(t *testing.T) {
	rec := &recorder{}
	p := authz.NewPolicy(roles{"places/maintainer": "maintainer"}, protection{"dev": false, "prod": true}, rec)
	a := via("maintainer", only(scope.Deploy, scope.Protected), places, true)

	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "prod"}))
	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"}))

	require.Len(t, rec.events, 1)
	require.Equal(t, "token.bypass_used", rec.events[0].Action)
	require.Equal(t, "tok", rec.events[0].TokenID)
	require.Equal(t, "prod", rec.events[0].Environment)
	require.Equal(t, "deploy", rec.events[0].Details["action"])
}

func Test_CanRead_NeedsTheReadScopeOnlyFromAToken(t *testing.T) {
	require.NoError(t, authz.CanRead(people["member"]))
	require.NoError(t, authz.CanRead(via("member", only(scope.Read), places, false)))
	requireCode(t, apperror.CodeMissingScope, authz.CanRead(via("member", only(scope.Deploy), places, false)))
	requireCode(t, apperror.CodeMissingScope, authz.CanRead(via("member", only(scope.Logs), places, false)))
}

func Test_Policy_TeamsAdminOnlyManagesTheTokensTeams(t *testing.T) {
	ctx := context.Background()
	a := via("admin", only(scope.TeamsAdmin), places, false)

	requireCode(t, "", policy.Can(ctx, a, authz.ManageTeams, authz.Resource{TeamID: "places"}))
	err := policy.Can(ctx, a, authz.ManageTeams, authz.Resource{TeamID: "reviews"})

	requireCode(t, apperror.CodeTeamNotGranted, err)
	require.Equal(t, "reviews", asAppError(t, err).Team)
}

func Test_Policy_TransfersStayInsideTheTokensTeams(t *testing.T) {
	ctx := context.Background()
	a := via("maintainer", only(scope.ServicesAdmin), []string{"places", "reviews"}, false)
	limited := via("maintainer", only(scope.ServicesAdmin), places, false)
	into := func(team string) authz.Resource { return authz.Resource{TeamID: "places", DestinationTeamID: team} }

	requireCode(t, "", policy.Can(ctx, a, authz.TransferService, into("reviews")))
	err := policy.Can(ctx, limited, authz.TransferService, into("reviews"))

	requireCode(t, apperror.CodeTeamNotGranted, err)
	require.Equal(t, "reviews", asAppError(t, err).Team)
}

func Test_Policy_SessionsTransferIntoAnyTeam(t *testing.T) {
	err := policy.Can(context.Background(), people["maintainer"], authz.TransferService, authz.Resource{TeamID: "places", DestinationTeamID: "reviews"})

	require.NoError(t, err)
}

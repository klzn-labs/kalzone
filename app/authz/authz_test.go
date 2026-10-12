package authz_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

type roles map[string]string

func (r roles) RoleOf(_ context.Context, teamID, userID string) (string, error) {
	return r[teamID+"/"+userID], nil
}

type protection map[string]bool

func (p protection) IsProtected(_ context.Context, env string) (bool, error) {
	return p[env], nil
}

type recorder struct{ events []events.Event }

func (r *recorder) Publish(_ context.Context, e events.Event) { r.events = append(r.events, e) }

var policy = authz.NewPolicy(
	roles{"places/member": "member", "places/maintainer": "maintainer"},
	protection{"dev": false, "prod": true},
	&recorder{},
)

var people = map[string]actor.Actor{
	"anonymous":  {},
	"outsider":   {UserID: "outsider", Login: "outsider"},
	"member":     {UserID: "member", Login: "member"},
	"maintainer": {UserID: "maintainer", Login: "maintainer"},
	"admin":      {UserID: "admin", Login: "admin", Admin: true},
}

type rule struct {
	action  authz.Action
	env     string
	allowed []string
}

var matrix = []rule{
	{authz.CreateService, "", []string{"member", "maintainer", "admin"}},
	{authz.EditService, "", []string{"member", "maintainer", "admin"}},
	{authz.CreateRelease, "", []string{"member", "maintainer", "admin"}},
	{authz.EditConfig, "dev", []string{"member", "maintainer", "admin"}},
	{authz.EditConfig, "prod", []string{"maintainer", "admin"}},
	{authz.EditSecrets, "dev", []string{"member", "maintainer", "admin"}},
	{authz.EditSecrets, "prod", []string{"maintainer", "admin"}},
	{authz.EditQueue, "dev", []string{"member", "maintainer", "admin"}},
	{authz.EditQueue, "prod", []string{"maintainer", "admin"}},
	{authz.Deploy, "dev", []string{"member", "maintainer", "admin"}},
	{authz.Deploy, "prod", []string{"maintainer", "admin"}},
	{authz.DeleteService, "", []string{"maintainer", "admin"}},
	{authz.TransferService, "", []string{"maintainer", "admin"}},
	{authz.ManageMembers, "", []string{"maintainer", "admin"}},
	{authz.ManageTeams, "", []string{"admin"}},
	{authz.ManageAdmins, "", []string{"admin"}},
}

func Test_Policy_Matrix(t *testing.T) {
	for _, r := range matrix {
		for name, person := range people {
			t.Run(fmt.Sprintf("%s %s by %s", r.action, r.env, name), checkRule(r, name, person))
		}
	}
}

func checkRule(r rule, name string, person actor.Actor) func(t *testing.T) {
	return func(t *testing.T) {
		err := policy.Can(context.Background(), person, r.action, authz.Resource{TeamID: "places", Environment: r.env})
		assertDecision(t, name, contains(r.allowed, name), err)
	}
}

func Test_Policy_RolesInAnotherTeamDoNotCount(t *testing.T) {
	err := policy.Can(context.Background(), people["maintainer"], authz.EditService, authz.Resource{TeamID: "reviews"})

	require.ErrorIs(t, err, apperror.Forbidden(""))
}

func Test_Policy_UnknownActionIsForbidden(t *testing.T) {
	err := policy.Can(context.Background(), people["maintainer"], authz.Action("nope"), authz.Resource{TeamID: "places"})

	require.ErrorIs(t, err, apperror.Forbidden(""))
}

func assertDecision(t *testing.T, name string, allowed bool, err error) {
	t.Helper()
	switch {
	case allowed:
		require.NoError(t, err)
	case name == "anonymous":
		require.ErrorIs(t, err, apperror.New(apperror.CodeUnauthenticated, ""))
	default:
		require.ErrorIs(t, err, apperror.Forbidden(""))
	}
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

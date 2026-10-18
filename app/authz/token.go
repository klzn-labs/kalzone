package authz

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

var scopeOf = map[Action]scope.Scope{
	CreateService: scope.Services, EditService: scope.Services, EditQueue: scope.Services,
	DeleteService: scope.ServicesAdmin, TransferService: scope.ServicesAdmin,
	CreateRelease: scope.Builds, EditConfig: scope.Config, EditSecrets: scope.Secrets, Deploy: scope.Deploy, ReadLogs: scope.Logs,
	ManageMembers: scope.TeamsAdmin, ManageTeams: scope.TeamsAdmin, ManageTokens: scope.TokensAdmin,
}

var protectable = map[Action]bool{EditConfig: true, EditSecrets: true, EditQueue: true, Deploy: true}

var (
	errNeverForTokens   = apperror.Forbidden("no token can do this, sign in to the website")
	errApprovalRequired = apperror.New(apperror.CodeApprovalRequired, "changes to a protected environment need a human's approval").
				WithHint("use a bypass token, or ask a human to do it in the browser")
)

func (p Policy) tokenGate(ctx context.Context, a actor.Actor, action Action, r Resource) (Gate, error) {
	needed, allowed := scopeOf[action]
	if !allowed {
		return "", errNeverForTokens
	}
	if !a.Token.Has(needed) {
		return "", apperror.MissingScope(string(needed))
	}
	if err := teamsGranted(a, r.TeamID, r.DestinationTeamID); err != nil {
		return "", err
	}
	if !protectable[action] {
		return Open, nil
	}
	return p.protectedRule(ctx, a, r)
}

func teamsGranted(a actor.Actor, teamIDs ...string) error {
	for _, teamID := range teamIDs {
		if teamID != "" && !a.Sees(teamID) {
			return apperror.TeamNotGranted(teamID)
		}
	}
	return nil
}

func (p Policy) protectedRule(ctx context.Context, a actor.Actor, r Resource) (Gate, error) {
	protected, err := p.environments.IsProtected(ctx, r.Environment)
	if err != nil {
		return "", err
	}
	if !protected {
		return Open, nil
	}
	if !a.Token.Has(scope.Protected) {
		return "", apperror.MissingScope(string(scope.Protected))
	}
	return protectedGate(a), nil
}

func protectedGate(a actor.Actor) Gate {
	switch {
	case a.Approved():
		return Approved
	case a.Token.Bypass:
		return Bypassed
	}
	return NeedsApproval
}

func PublishBypass(ctx context.Context, p Publisher, g Gate, a actor.Actor, action Action, r Resource) {
	if g == Bypassed {
		p.Publish(ctx, bypassEvent(a, action, r))
	}
}

func bypassEvent(a actor.Actor, action Action, r Resource) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name,
		Action: "token.bypass_used", TargetKind: events.TargetToken, TargetID: a.Token.ID, TargetName: a.Token.Name,
		TeamID: r.TeamID, Environment: r.Environment, Details: map[string]any{"action": string(action)},
	}
}

func CanRead(a actor.Actor) error {
	if a.ViaToken() && !a.Token.Has(scope.Read) {
		return apperror.MissingScope(string(scope.Read))
	}
	return nil
}

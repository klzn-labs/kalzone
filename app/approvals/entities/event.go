package entities

import (
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/events"
)

func (r Request) ApprovePath() string {
	return "/requests/" + r.ID
}

func (r Request) PollPath() string {
	return "/api/v1/requests/" + r.ID
}

func EventOf(a actor.Actor, action string, r Request) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetRequest, TargetID: r.ID, TargetName: r.ServiceSlug, TeamID: r.TeamID, Environment: r.Environment,
		Details: map[string]any{"kind": r.Kind, "summary": r.Summary, "reason": r.Reason, "result": r.Result, "error": r.ErrorMessage},
	}
}

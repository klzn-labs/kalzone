package events

import (
	"context"
	"time"

	"github.com/adrianozp/platform/pkg/actor"
)

type Event struct {
	At          time.Time
	ActorID     string
	ActorLogin  string
	TokenID     string
	TokenName   string
	Action      string
	TargetKind  string
	TargetID    string
	TargetName  string
	TeamID      string
	Environment string
	Details     map[string]any
}

type Subscriber interface {
	Handle(ctx context.Context, event Event) error
}

const (
	TargetTeam       = "team"
	TargetUser       = "user"
	TargetService    = "service"
	TargetConfig     = "config"
	TargetRelease    = "release"
	TargetDeployment = "deployment"
	TargetQueue      = "queue"
	TargetToken      = "token"
	TargetRequest    = "request"
)

func WithApproval(a actor.Actor, details map[string]any) map[string]any {
	if !a.Approved() {
		return details
	}
	if details == nil {
		details = map[string]any{}
	}
	details["request_id"], details["approved_by"] = a.Approval.RequestID, a.Approval.ApproverLogin
	return details
}

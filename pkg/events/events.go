package events

import (
	"context"
	"time"
)

type Event struct {
	At          time.Time
	ActorID     string
	ActorLogin  string
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
	TargetTeam    = "team"
	TargetUser    = "user"
	TargetService = "service"
	TargetConfig  = "config"
)

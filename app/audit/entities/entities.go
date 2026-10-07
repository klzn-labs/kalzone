package entities

import "time"

type Event struct {
	ID          string
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

type Filter struct {
	TeamID      string
	TargetID    string
	ActorID     string
	Environment string
	Before      *time.Time
	Limit       int
}

type Query struct {
	Team        string
	Service     string
	Actor       string
	Environment string
	Before      *time.Time
	Limit       int
}

package entities

import "time"

type Event struct {
	ID          string
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

type Filter struct {
	TeamID      string
	TargetID    string
	ActorID     string
	TokenID     string
	TeamIDs     []string
	AllTeams    bool
	ViewerID    string
	Environment string
	Before      *time.Time
	BeforeID    string
	Limit       int
}

type Query struct {
	Team        string
	Service     string
	Actor       string
	Token       string
	Environment string
	Before      *time.Time
	Limit       int
}

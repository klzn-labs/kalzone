package entities

import "time"

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusSucceeded  Status = "succeeded"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

type Trigger string

const (
	TriggerManual    Trigger = "manual"
	TriggerPromote   Trigger = "promote"
	TriggerRollback  Trigger = "rollback"
	TriggerConfig    Trigger = "config"
	TriggerReconcile Trigger = "reconcile"
)

type Strategy string

const (
	StrategyBlueGreen Strategy = "bluegreen"
	StrategyCanary    Strategy = "canary"
)

type Deployment struct {
	ID             string
	ServiceID      string
	Environment    string
	Number         int
	ReleaseID      string
	ReleaseNumber  int
	ImageRef       string
	ConfigRevision int
	Strategy       Strategy
	Trigger        Trigger
	Status         Status
	RequestedBy    string
	Agent          string
	Message        string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	Failure        *Failure
}

func (d Deployment) Active() bool {
	return d.Status == StatusPending || d.Status == StatusInProgress
}

type Event struct {
	ID           int64
	DeploymentID string
	At           time.Time
	Message      string
}

type Agent struct {
	Name       string
	Version    string
	LastSeenAt time.Time
}

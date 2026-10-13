package mysql

import (
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
)

type deployment struct {
	ID             string `gorm:"primaryKey"`
	ServiceID      string
	Environment    string
	Number         int
	ReleaseID      string
	ReleaseNumber  int
	ImageRef       string
	ConfigRevision int
	Strategy       string
	Trigger        string `gorm:"column:trigger"`
	Status         string
	RequestedBy    string
	Agent          string
	Message        string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

func (deployment) TableName() string { return "deployments" }

func fromDeployment(d entities.Deployment) deployment {
	return deployment{
		ID: d.ID, ServiceID: d.ServiceID, Environment: d.Environment, Number: d.Number, ReleaseID: d.ReleaseID,
		ReleaseNumber: d.ReleaseNumber, ImageRef: d.ImageRef, ConfigRevision: d.ConfigRevision, Strategy: string(d.Strategy),
		Trigger: string(d.Trigger), Status: string(d.Status), RequestedBy: d.RequestedBy, Agent: d.Agent, Message: d.Message,
		CreatedAt: d.CreatedAt, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
	}
}

func (m deployment) toEntity() entities.Deployment {
	return entities.Deployment{
		ID: m.ID, ServiceID: m.ServiceID, Environment: m.Environment, Number: m.Number, ReleaseID: m.ReleaseID,
		ReleaseNumber: m.ReleaseNumber, ImageRef: m.ImageRef, ConfigRevision: m.ConfigRevision, Strategy: entities.Strategy(m.Strategy),
		Trigger: entities.Trigger(m.Trigger), Status: entities.Status(m.Status), RequestedBy: m.RequestedBy, Agent: m.Agent,
		Message: m.Message, CreatedAt: m.CreatedAt, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt,
	}
}

type deploymentEvent struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	DeploymentID string
	At           time.Time
	Message      string
}

func (deploymentEvent) TableName() string { return "deployment_events" }

func (m deploymentEvent) toEntity() entities.Event {
	return entities.Event{ID: m.ID, DeploymentID: m.DeploymentID, At: m.At, Message: m.Message}
}

type agent struct {
	Name       string `gorm:"primaryKey"`
	Version    string
	LastSeenAt time.Time
}

func (agent) TableName() string { return "agents" }

package deployments

import (
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
)

type DeploymentDTO struct {
	ID             string     `json:"id"`
	Environment    string     `json:"environment"`
	Number         int        `json:"number"`
	ReleaseID      string     `json:"release_id"`
	ReleaseNumber  int        `json:"release_number"`
	ImageRef       string     `json:"image_ref"`
	ConfigRevision int        `json:"config_revision"`
	Strategy       string     `json:"strategy"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	Agent          string     `json:"agent,omitempty"`
	Message        string     `json:"message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

func ToDTO(d entities.Deployment) DeploymentDTO {
	return DeploymentDTO{
		ID: d.ID, Environment: d.Environment, Number: d.Number, ReleaseID: d.ReleaseID, ReleaseNumber: d.ReleaseNumber,
		ImageRef: d.ImageRef, ConfigRevision: d.ConfigRevision, Strategy: string(d.Strategy), Trigger: string(d.Trigger),
		Status: string(d.Status), Agent: d.Agent, Message: d.Message, CreatedAt: d.CreatedAt, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
	}
}

type eventDTO struct {
	At      time.Time `json:"at"`
	Message string    `json:"message"`
}

type deployDTO struct {
	Service     string `json:"service" binding:"required"`
	Environment string `json:"environment" binding:"required"`
	ReleaseID   string `json:"release_id" binding:"required"`
	Strategy    string `json:"strategy"`
}

type promoteDTO struct {
	Service string `json:"service" binding:"required"`
	From    string `json:"from" binding:"required"`
	To      string `json:"to" binding:"required"`
}

type applyDTO struct {
	Service     string `json:"service" binding:"required"`
	Environment string `json:"environment" binding:"required"`
}

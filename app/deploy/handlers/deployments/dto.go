package deployments

import (
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/internal/apispec"
)

type DeploymentDTO struct {
	ID             string      `json:"id"`
	Environment    string      `json:"environment"`
	Number         int         `json:"number"`
	ReleaseID      string      `json:"release_id"`
	ReleaseNumber  int         `json:"release_number"`
	ImageRef       string      `json:"image_ref"`
	ConfigRevision int         `json:"config_revision"`
	Strategy       string      `json:"strategy" enum:"bluegreen,canary"`
	Trigger        string      `json:"trigger" enum:"manual,promote,rollback,config,reconcile"`
	Status         string      `json:"status" enum:"pending,in_progress,succeeded,failed,cancelled"`
	Agent          string      `json:"agent,omitempty"`
	Message        string      `json:"message,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
	EventsSummary  *summaryDTO `json:"events_summary,omitempty"`
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

func toEventDTO(e entities.Event) eventDTO {
	return eventDTO{At: e.At, Message: e.Message}
}

type phaseDTO struct {
	Name    string `json:"name" enum:"waiting,running"`
	Seconds int    `json:"seconds"`
}

type summaryDTO struct {
	Total         int        `json:"total"`
	ProgressLines int        `json:"progress_lines"`
	Phases        []phaseDTO `json:"phases"`
	Tail          []eventDTO `json:"tail"`
	Log           string     `json:"log"`
}

func withSummary(d entities.Deployment, s entities.Summary) DeploymentDTO {
	dto := ToDTO(d)
	summary := summaryDTO{Total: s.Total, ProgressLines: s.ProgressLines, Phases: []phaseDTO{}, Tail: []eventDTO{}, Log: apispec.BasePath + "/deployments/" + d.ID + "/events"}
	for _, p := range s.Phases {
		summary.Phases = append(summary.Phases, phaseDTO(p))
	}
	for _, e := range s.Tail {
		summary.Tail = append(summary.Tail, toEventDTO(e))
	}
	dto.EventsSummary = &summary
	return dto
}

type deployDTO struct {
	Service     string `json:"service" binding:"required"`
	Environment string `json:"environment" binding:"required"`
	Release     string `json:"release" binding:"required" description:"the release id"`
	Strategy    string `json:"strategy,omitempty" enum:"bluegreen,canary"`
}

func (d deployDTO) toRequest() deployments.Request {
	return deployments.Request{Service: d.Service, Environment: d.Environment, ReleaseID: d.Release, Strategy: d.Strategy}
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

type listParams struct {
	Service     string `query:"service" required:"true" description:"service slug"`
	Environment string `query:"env" required:"true" description:"environment slug"`
	apispec.ListParams
}

type deploymentParams struct {
	ID string `path:"id"`
}

type eventsParams struct {
	ID   string `path:"id"`
	Tail int    `query:"tail" minimum:"1" maximum:"100" description:"the last N lines; not with cursor"`
	apispec.ListParams
}

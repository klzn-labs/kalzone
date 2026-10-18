package deployments

import (
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/apperror"
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
	Trigger        string      `json:"trigger" enum:"manual,promote,rollback,config,reconcile,push"`
	Status         string      `json:"status" enum:"pending,in_progress,succeeded,failed,cancelled"`
	Agent          string      `json:"agent,omitempty"`
	Message        string      `json:"message,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
	EventsSummary  *summaryDTO `json:"events_summary,omitempty"`
	Failure        *failureDTO `json:"failure,omitempty"`
}

type failureDTO struct {
	Class     string   `json:"class" enum:"timeout,oom_killed,unhealthy,crashed,start_failed,smoke_failed,gate_failed,platform_error"`
	Hint      string   `json:"hint"`
	Container string   `json:"container"`
	ExitCode  int      `json:"exit_code"`
	OOMKilled bool     `json:"oom_killed"`
	HealthLog []string `json:"health_log"`
	Logs      []string `json:"logs"`
}

func toFailureDTO(f *entities.Failure) *failureDTO {
	if f == nil {
		return nil
	}
	return &failureDTO{
		Class: f.Class, Hint: f.Hint(), Container: f.Container, ExitCode: f.ExitCode, OOMKilled: f.OOMKilled,
		HealthLog: nonNil(f.HealthLog), Logs: nonNil(f.Logs),
	}
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}

func ToDTO(d entities.Deployment) DeploymentDTO {
	return DeploymentDTO{
		ID: d.ID, Environment: d.Environment, Number: d.Number, ReleaseID: d.ReleaseID, ReleaseNumber: d.ReleaseNumber,
		ImageRef: d.ImageRef, ConfigRevision: d.ConfigRevision, Strategy: string(d.Strategy), Trigger: string(d.Trigger),
		Status: string(d.Status), Agent: d.Agent, Message: d.Message, CreatedAt: d.CreatedAt, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
		Failure: toFailureDTO(d.Failure),
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

var errBothForms = apperror.Invalid("send one target or a list of targets, not both").WithField("targets").
	WithHint("leave out service and the environment when sending targets")

type targetDTO struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
}

type deployDTO struct {
	Service     string      `json:"service,omitempty" description:"the service of a single deployment; leave out with targets"`
	Environment string      `json:"environment,omitempty" description:"the environment of a single deployment; leave out with targets"`
	Targets     []targetDTO `json:"targets,omitempty" minItems:"1" maxItems:"10" description:"1 to 10 services and environments, deployed all or nothing"`
	Release     string      `json:"release" binding:"required" description:"a release id, latest, r42, sha:<7 to 40 hex> or a tag, resolved per service"`
	Strategy    string      `json:"strategy,omitempty" enum:"bluegreen,canary"`
}

func (d deployDTO) batch() (bool, error) {
	return batchForm(d.Service, d.Environment, "environment", d.Targets)
}

func (d deployDTO) toRequest() deployments.Request {
	return deployments.Request{Service: d.Service, Environment: d.Environment, ReleaseID: d.Release, Strategy: d.Strategy}
}

func (d deployDTO) toBatch() deployments.Batch {
	return deployments.Batch{Targets: toTargets(d.Targets), Release: d.Release, Strategy: d.Strategy}
}

type promoteDTO struct {
	Service string      `json:"service,omitempty" description:"the service of a single promotion; leave out with targets"`
	To      string      `json:"to,omitempty" description:"the environment of a single promotion; leave out with targets"`
	Targets []targetDTO `json:"targets,omitempty" minItems:"1" maxItems:"10" description:"1 to 10 services and the environments to promote them to, all or nothing"`
	From    string      `json:"from" binding:"required"`
}

func (d promoteDTO) batch() (bool, error) {
	return batchForm(d.Service, d.To, "to", d.Targets)
}

func batchForm(service, environment, environmentField string, targets []targetDTO) (bool, error) {
	switch {
	case targets != nil && (service != "" || environment != ""):
		return false, errBothForms
	case targets != nil:
		return true, nil
	case service == "":
		return false, missing("service", environmentField)
	case environment == "":
		return false, missing(environmentField, environmentField)
	}
	return false, nil
}

func missing(field, environmentField string) error {
	return apperror.Invalid(field + " is required").WithField(field).
		WithHint("send service and " + environmentField + ", or send targets")
}

func toTargets(targets []targetDTO) []deployments.Target {
	result := make([]deployments.Target, 0, len(targets))
	for _, t := range targets {
		result = append(result, deployments.Target(t))
	}
	return result
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

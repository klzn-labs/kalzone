package crons

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/cronspec"
)

const runNote = "the gateway starts the run within a few seconds; follow it with GET /api/v1/crons/{env}/{name}/runs"

type RunDTO struct {
	ID          string    `json:"id"`
	Manual      bool      `json:"manual"`
	Status      string    `json:"status" enum:"succeeded,failed,timed_out,skipped"`
	ScheduledAt time.Time `json:"scheduled_at"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Attempts    int       `json:"attempts"`
	LastStatus  int       `json:"last_status" description:"HTTP status of the last attempt; 0 when it got no answer"`
	Error       string    `json:"error,omitempty"`
}

type CronDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Environment string          `json:"environment"`
	TeamID      string          `json:"team_id"`
	Service     string          `json:"service"`
	Schedule    string          `json:"schedule"`
	Path        string          `json:"path"`
	Payload     json.RawMessage `json:"payload"`
	Timeout     string          `json:"timeout"`
	Paused      bool            `json:"paused"`
	NextRunAt   *time.Time      `json:"next_run_at,omitempty" description:"next scheduled call in UTC; absent while paused"`
	LastRun     *RunDTO         `json:"last_run,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type runRequestedDTO struct {
	RunID string `json:"run_id"`
	Note  string `json:"note"`
}

type createDTO struct {
	Name        string          `json:"name" binding:"required"`
	Environment string          `json:"environment" binding:"required"`
	Service     string          `json:"service" binding:"required" description:"the service to call; the cron belongs to its owner team"`
	Schedule    string          `json:"schedule" binding:"required" description:"five cron fields in UTC, minute hour day-of-month month day-of-week, or @hourly, @daily, @weekly, @monthly"`
	Path        string          `json:"path" binding:"required" description:"path the platform POSTs to on the service's internal address, starting with /"`
	Payload     json.RawMessage `json:"payload,omitempty" description:"fixed JSON body of every call, at most 64 KiB; {} when left out"`
	Timeout     string          `json:"timeout,omitempty" description:"time one run may take, retries included, from 1s to 15m; 60s when left out"`
}

type changeDTO struct {
	Schedule *string         `json:"schedule,omitempty" description:"five cron fields in UTC, or @hourly, @daily, @weekly, @monthly"`
	Path     *string         `json:"path,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty" description:"replaces the JSON body; {} sends an empty object"`
	Timeout  *string         `json:"timeout,omitempty" description:"from 1s to 15m"`
}

type described struct {
	cron    entities.Cron
	service string
	lastRun *entities.Run
}

func (d createDTO) draft() (crons.Draft, error) {
	timeout, err := parseTimeout(d.Timeout)
	return crons.Draft{
		Name: d.Name, Environment: d.Environment, Service: d.Service, Schedule: d.Schedule,
		Path: d.Path, Payload: d.Payload, Timeout: timeout,
	}, err
}

func (d changeDTO) change() (crons.Change, error) {
	change := crons.Change{Schedule: d.Schedule, Path: d.Path}
	if d.Payload != nil {
		payload := []byte(d.Payload)
		change.Payload = &payload
	}
	if d.Timeout == nil {
		return change, nil
	}
	timeout, err := parseTimeout(*d.Timeout)
	change.Timeout = &timeout
	return change, err
}

func parseTimeout(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, apperror.Invalid("timeout must be a duration such as 30s or 5m").WithField("timeout")
	}
	return timeout, nil
}

func toDTO(d described, now time.Time) CronDTO {
	c := d.cron
	return CronDTO{
		ID: c.ID, Name: c.Name, Environment: c.Environment, TeamID: c.TeamID, Service: d.service, Schedule: c.Schedule,
		Path: c.Path, Payload: c.Body(), Timeout: c.Timeout.String(), Paused: c.Paused, NextRunAt: nextRun(c, now),
		LastRun: lastRunDTO(d.lastRun), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func nextRun(c entities.Cron, now time.Time) *time.Time {
	schedule, err := cronspec.Parse(c.Schedule)
	if err != nil || c.Paused {
		return nil
	}
	next := schedule.Next(now)
	return &next
}

func lastRunDTO(run *entities.Run) *RunDTO {
	if run == nil {
		return nil
	}
	dto := toRunDTO(*run)
	return &dto
}

func toRunDTO(r entities.Run) RunDTO {
	return RunDTO{
		ID: r.ID, Manual: r.Manual, Status: r.Status, ScheduledAt: r.ScheduledAt, StartedAt: r.StartedAt,
		FinishedAt: r.FinishedAt, Attempts: r.Attempts, LastStatus: r.LastStatus, Error: r.Error,
	}
}

func describedKey(d described) string {
	return d.cron.Environment + "/" + d.cron.Name
}

func runKey(r entities.Run) string {
	return r.ID
}

package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
)

type cronModel struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Environment string
	TeamID      string
	ServiceID   string
	Schedule    string
	Path        string
	Payload     []byte
	TimeoutMS   int64 `gorm:"column:timeout_ms"`
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time `gorm:"autoUpdateTime:false"`
}

func (cronModel) TableName() string { return "crons" }

type runModel struct {
	ID          string `gorm:"primaryKey"`
	CronID      string
	Manual      bool `gorm:"column:on_demand"`
	Status      string
	ScheduledAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	Attempts    int
	LastStatus  int
	Error       string
}

func (runModel) TableName() string { return "cron_runs" }

type commandModel struct {
	ID        string `gorm:"primaryKey"`
	CronID    string
	Kind      string
	CreatedAt time.Time
	DoneAt    *time.Time
}

func (commandModel) TableName() string { return "cron_commands" }

func fromCron(c entities.Cron) cronModel {
	return cronModel{
		ID: c.ID, Name: c.Name, Environment: c.Environment, TeamID: c.TeamID, ServiceID: c.ServiceID,
		Schedule: c.Schedule, Path: c.Path, Payload: c.Payload, TimeoutMS: c.Timeout.Milliseconds(),
		Paused: c.Paused, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func (m cronModel) toEntity() entities.Cron {
	return entities.Cron{
		ID: m.ID, Name: m.Name, Environment: m.Environment, TeamID: m.TeamID, ServiceID: m.ServiceID,
		Schedule: m.Schedule, Path: m.Path, Payload: m.Payload, Timeout: time.Duration(m.TimeoutMS) * time.Millisecond,
		Paused: m.Paused, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fromRun(r entities.Run) runModel {
	return runModel(r)
}

func (m runModel) toEntity() entities.Run {
	return entities.Run(m)
}

func fromCommand(c entities.Command) commandModel {
	return commandModel(c)
}

func (m commandModel) toEntity() entities.Command {
	return entities.Command(m)
}

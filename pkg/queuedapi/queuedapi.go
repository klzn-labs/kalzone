package queuedapi

import (
	"encoding/json"
	"time"
)

const (
	DesiredPath  = "/queued/v1/desired"
	StatsPath    = "/queued/v1/stats"
	CronRunsPath = "/queued/v1/cron-runs"
)

func DonePath(commandID string) string {
	return "/queued/v1/commands/" + commandID + "/done"
}

type Backoff struct {
	Kind    string `json:"kind"`
	Initial string `json:"initial"`
	Max     string `json:"max"`
}

type Settings struct {
	Path             string  `json:"path"`
	RatePerSecond    float64 `json:"rate_per_second"`
	Adaptive         bool    `json:"adaptive,omitempty"`
	MinRatePerSecond float64 `json:"min_rate_per_second,omitempty"`
	MaxInFlight      int     `json:"max_in_flight"`
	MaxAttempts      int     `json:"max_attempts"`
	Backoff          Backoff `json:"backoff"`
	DeliveryTimeout  string  `json:"delivery_timeout"`
	Retention        string  `json:"retention"`
	MaxMessageBytes  int     `json:"max_message_bytes"`
}

type Queue struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Environment     string   `json:"environment"`
	Version         int      `json:"version"`
	Settings        Settings `json:"settings"`
	Paused          bool     `json:"paused"`
	Target          string   `json:"target"`
	PublisherHashes []string `json:"publisher_hashes"`
	SigningKey      string   `json:"signing_key"`
	PreviousKey     string   `json:"previous_key,omitempty"`
}

type Cron struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Environment string          `json:"environment"`
	Service     string          `json:"service"`
	Schedule    string          `json:"schedule"`
	Target      string          `json:"target"`
	Payload     json.RawMessage `json:"payload"`
	Timeout     string          `json:"timeout"`
	Paused      bool            `json:"paused"`
	SigningKey  string          `json:"signing_key"`
	PreviousKey string          `json:"previous_key,omitempty"`
}

type Command struct {
	ID       string `json:"id"`
	QueueID  string `json:"queue_id"`
	CronID   string `json:"cron_id,omitempty"`
	Kind     string `json:"kind"`
	Argument string `json:"argument,omitempty"`
}

type Desired struct {
	Queues   []Queue   `json:"queues"`
	Crons    []Cron    `json:"crons"`
	Commands []Command `json:"commands"`
}

type CronRun struct {
	ID          string    `json:"id"`
	CronID      string    `json:"cron_id"`
	Manual      bool      `json:"manual"`
	Status      string    `json:"status"`
	ScheduledAt time.Time `json:"scheduled_at"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Attempts    int       `json:"attempts"`
	LastStatus  int       `json:"last_status"`
	Error       string    `json:"error,omitempty"`
}

type CronRunReport struct {
	Runs []CronRun `json:"runs"`
}

type DeadLetter struct {
	ID          string    `json:"id"`
	Attempts    int       `json:"attempts"`
	LastStatus  int       `json:"last_status"`
	LastError   string    `json:"last_error"`
	PublishedAt time.Time `json:"published_at"`
	BodyPreview string    `json:"body_preview"`
}

type Stats struct {
	QueueID            string       `json:"queue_id"`
	Depth              int64        `json:"depth"`
	InFlight           int          `json:"in_flight"`
	OldestAgeSeconds   float64      `json:"oldest_age_seconds"`
	DeliveredPerMinute float64      `json:"delivered_per_minute"`
	FailedPerMinute    float64      `json:"failed_per_minute"`
	DeadLetters        int64        `json:"dead_letters"`
	NewestDead         []DeadLetter `json:"newest_dead"`
	EffectiveRate      float64      `json:"effective_rate_per_second"`
	Adaptive           *Adaptive    `json:"adaptive,omitempty"`
}

type Adaptive struct {
	Factor      float64 `json:"factor"`
	DatabaseCPU float64 `json:"database_cpu"`
	TargetCPU   float64 `json:"target_cpu"`
	FailSafe    bool    `json:"fail_safe"`
}

type StatsReport struct {
	Stats []Stats `json:"stats"`
}

package entities

import "time"

const (
	DefaultTimeout = time.Minute
	KeptRuns       = 50
	CommandRun     = "run"
)

const (
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunTimedOut  = "timed_out"
	RunSkipped   = "skipped"
)

var emptyPayload = []byte("{}")

type Cron struct {
	ID          string
	Name        string
	Environment string
	TeamID      string
	ServiceID   string
	Schedule    string
	Path        string
	Payload     []byte
	Timeout     time.Duration
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c Cron) Body() []byte {
	if len(c.Payload) == 0 {
		return emptyPayload
	}
	return c.Payload
}

type Run struct {
	ID          string
	CronID      string
	Manual      bool
	Status      string
	ScheduledAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	Attempts    int
	LastStatus  int
	Error       string
}

type Command struct {
	ID        string
	CronID    string
	Kind      string
	CreatedAt time.Time
	DoneAt    *time.Time
}

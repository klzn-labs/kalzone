package entities

import "time"

const (
	CommandReplay    = "replay"
	CommandReplayAll = "replay_all"
	CommandPurgeDead = "purge_dead"
	CommandDelete    = "delete"
)

type Command struct {
	ID        string
	QueueID   string
	Kind      string
	Argument  string
	CreatedAt time.Time
	DoneAt    *time.Time
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
	ReportedAt         time.Time    `json:"reported_at"`
}

const StatsFreshFor = 60 * time.Second

func (s Stats) Fresh(now time.Time) bool {
	return !s.ReportedAt.IsZero() && now.Sub(s.ReportedAt) <= StatsFreshFor
}

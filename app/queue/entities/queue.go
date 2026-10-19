package entities

import (
	"slices"
	"time"
)

const (
	BackoffFixed       = "fixed"
	BackoffExponential = "exponential"
)

type Backoff struct {
	Kind    string        `json:"kind"`
	Initial time.Duration `json:"initial"`
	Max     time.Duration `json:"max"`
}

type Settings struct {
	Path            string        `json:"path"`
	RatePerSecond   float64       `json:"rate_per_second"`
	MaxInFlight     int           `json:"max_in_flight"`
	MaxAttempts     int           `json:"max_attempts"`
	Backoff         Backoff       `json:"backoff"`
	DeliveryTimeout time.Duration `json:"delivery_timeout"`
	Retention       time.Duration `json:"retention"`
	MaxMessageBytes int           `json:"max_message_bytes"`
}

type Queue struct {
	ID          string
	Name        string
	Environment string
	TeamID      string
	ConsumerID  string
	Version     int
	Settings    Settings
	Publishers  []string
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (q Queue) Attached() []string {
	return slices.DeleteFunc(slices.Clone(q.Publishers), func(id string) bool { return id == q.ConsumerID })
}

type Version struct {
	QueueID   string
	Number    int
	Settings  Settings
	AuthorID  string
	Message   string
	CreatedAt time.Time
}

func DefaultSettings(path string) Settings {
	return Settings{
		Path: path, RatePerSecond: 10, MaxInFlight: 4, MaxAttempts: 5,
		Backoff:         Backoff{Kind: BackoffExponential, Initial: time.Second, Max: 5 * time.Minute},
		DeliveryTimeout: 30 * time.Second, Retention: 7 * 24 * time.Hour, MaxMessageBytes: 256 << 10,
	}
}

package queues

import (
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type backoffDTO struct {
	Kind    string `json:"kind"`
	Initial string `json:"initial"`
	Max     string `json:"max"`
}

type settingsDTO struct {
	Path            string     `json:"path"`
	RatePerSecond   float64    `json:"rate_per_second"`
	MaxInFlight     int        `json:"max_in_flight"`
	MaxAttempts     int        `json:"max_attempts"`
	Backoff         backoffDTO `json:"backoff"`
	DeliveryTimeout string     `json:"delivery_timeout"`
	Retention       string     `json:"retention"`
	MaxMessageBytes int        `json:"max_message_bytes"`
}

type QueueDTO struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Environment string      `json:"environment"`
	TeamID      string      `json:"team_id"`
	Consumer    string      `json:"consumer"`
	Publishers  []string    `json:"publishers"`
	Version     int         `json:"version"`
	Settings    settingsDTO `json:"settings"`
	Paused      bool        `json:"paused"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type versionDTO struct {
	Number    int         `json:"number"`
	Settings  settingsDTO `json:"settings"`
	AuthorID  string      `json:"author_id"`
	Message   string      `json:"message"`
	CreatedAt time.Time   `json:"created_at"`
}

type createDTO struct {
	Name        string       `json:"name" binding:"required"`
	Environment string       `json:"environment" binding:"required"`
	Consumer    string       `json:"consumer" binding:"required"`
	Path        string       `json:"path" binding:"required"`
	Settings    *settingsDTO `json:"settings,omitempty" description:"initial settings; its path is ignored on create, the top-level path wins"`
	Publishers  []string     `json:"publishers,omitempty"`
	Message     string       `json:"message,omitempty"`
}

type changeDTO struct {
	Settings   *settingsDTO `json:"settings,omitempty"`
	Publishers *[]string    `json:"publishers,omitempty"`
	Message    string       `json:"message,omitempty"`
	Version    int          `json:"version" binding:"required" description:"the version you are changing from"`
}

type statsDTO struct {
	entities.Stats
	Fresh bool `json:"fresh"`
}

func toStatsDTO(s entities.Stats, fresh bool) statsDTO {
	if s.NewestDead == nil {
		s.NewestDead = []entities.DeadLetter{}
	}
	return statsDTO{Stats: s, Fresh: fresh}
}

func fromSettings(s entities.Settings) settingsDTO {
	return settingsDTO{
		Path: s.Path, RatePerSecond: s.RatePerSecond, MaxInFlight: s.MaxInFlight, MaxAttempts: s.MaxAttempts,
		Backoff:         backoffDTO{Kind: s.Backoff.Kind, Initial: s.Backoff.Initial.String(), Max: s.Backoff.Max.String()},
		DeliveryTimeout: s.DeliveryTimeout.String(), Retention: s.Retention.String(), MaxMessageBytes: s.MaxMessageBytes,
	}
}

func toSettings(d settingsDTO) (entities.Settings, error) {
	durations, err := parseDurations(map[string]string{
		"backoff.initial": d.Backoff.Initial, "backoff.max": d.Backoff.Max,
		"delivery_timeout": d.DeliveryTimeout, "retention": d.Retention,
	})
	if err != nil {
		return entities.Settings{}, err
	}
	return entities.Settings{
		Path: d.Path, RatePerSecond: d.RatePerSecond, MaxInFlight: d.MaxInFlight, MaxAttempts: d.MaxAttempts,
		Backoff:         entities.Backoff{Kind: d.Backoff.Kind, Initial: durations["backoff.initial"], Max: durations["backoff.max"]},
		DeliveryTimeout: durations["delivery_timeout"], Retention: durations["retention"], MaxMessageBytes: d.MaxMessageBytes,
	}, nil
}

func parseDurations(fields map[string]string) (map[string]time.Duration, error) {
	result := make(map[string]time.Duration, len(fields))
	for name, raw := range fields {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, apperror.Invalid(name + " must be a duration such as 30s, 5m or 24h")
		}
		result[name] = d
	}
	return result, nil
}

func (d createDTO) settings() (entities.Settings, error) {
	if d.Settings == nil {
		return entities.Settings{Path: d.Path}, nil
	}
	s, err := toSettings(*d.Settings)
	s.Path = d.Path
	return s, err
}

func (d changeDTO) settings() (*entities.Settings, error) {
	if d.Settings == nil {
		return nil, nil
	}
	s, err := toSettings(*d.Settings)
	return &s, err
}

func toVersionDTO(v entities.Version) versionDTO {
	return versionDTO{Number: v.Number, Settings: fromSettings(v.Settings), AuthorID: v.AuthorID, Message: v.Message, CreatedAt: v.CreatedAt}
}

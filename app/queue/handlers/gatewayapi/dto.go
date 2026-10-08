package gatewayapi

import (
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/usecases/gateway"
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

type desiredQueueDTO struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Environment     string      `json:"environment"`
	Version         int         `json:"version"`
	Settings        settingsDTO `json:"settings"`
	Paused          bool        `json:"paused"`
	Target          string      `json:"target"`
	PublisherHashes []string    `json:"publisher_hashes"`
	SigningKey      string      `json:"signing_key"`
	PreviousKey     string      `json:"previous_key,omitempty"`
}

type commandDTO struct {
	ID       string `json:"id"`
	QueueID  string `json:"queue_id"`
	Kind     string `json:"kind"`
	Argument string `json:"argument,omitempty"`
}

type desiredDTO struct {
	Queues   []desiredQueueDTO `json:"queues"`
	Commands []commandDTO      `json:"commands"`
}

func toDesiredDTO(d gateway.Desired) desiredDTO {
	result := desiredDTO{Queues: make([]desiredQueueDTO, 0, len(d.Queues)), Commands: make([]commandDTO, 0, len(d.Commands))}
	for _, q := range d.Queues {
		result.Queues = append(result.Queues, toQueueDTO(q))
	}
	for _, c := range d.Commands {
		result.Commands = append(result.Commands, commandDTO{ID: c.ID, QueueID: c.QueueID, Kind: c.Kind, Argument: c.Argument})
	}
	return result
}

func toQueueDTO(q gateway.DesiredQueue) desiredQueueDTO {
	return desiredQueueDTO{
		ID: q.ID, Name: q.Name, Environment: q.Environment, Version: q.Version, Settings: fromSettings(q.Settings),
		Paused: q.Paused, Target: q.Target, PublisherHashes: q.PublisherHashes, SigningKey: q.SigningKey, PreviousKey: q.PreviousKey,
	}
}

func fromSettings(s entities.Settings) settingsDTO {
	return settingsDTO{
		Path: s.Path, RatePerSecond: s.RatePerSecond, MaxInFlight: s.MaxInFlight, MaxAttempts: s.MaxAttempts,
		Backoff:         backoffDTO{Kind: s.Backoff.Kind, Initial: s.Backoff.Initial.String(), Max: s.Backoff.Max.String()},
		DeliveryTimeout: s.DeliveryTimeout.String(), Retention: s.Retention.String(), MaxMessageBytes: s.MaxMessageBytes,
	}
}

package gatewayapi

import (
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/gateway"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

func toDesiredDTO(d gateway.Desired) queuedapi.Desired {
	result := queuedapi.Desired{Queues: make([]queuedapi.Queue, 0, len(d.Queues)), Commands: make([]queuedapi.Command, 0, len(d.Commands))}
	for _, q := range d.Queues {
		result.Queues = append(result.Queues, toQueueDTO(q))
	}
	for _, c := range d.Commands {
		result.Commands = append(result.Commands, queuedapi.Command{ID: c.ID, QueueID: c.QueueID, Kind: c.Kind, Argument: c.Argument})
	}
	return result
}

func toQueueDTO(q gateway.DesiredQueue) queuedapi.Queue {
	return queuedapi.Queue{
		ID: q.ID, Name: q.Name, Environment: q.Environment, Version: q.Version, Settings: fromSettings(q.Settings),
		Paused: q.Paused, Target: q.Target, PublisherHashes: q.PublisherHashes, SigningKey: q.SigningKey, PreviousKey: q.PreviousKey,
	}
}

func fromSettings(s entities.Settings) queuedapi.Settings {
	return queuedapi.Settings{
		Path: s.Path, RatePerSecond: s.RatePerSecond, Adaptive: s.Adaptive, MinRatePerSecond: s.MinRatePerSecond,
		MaxInFlight: s.MaxInFlight, MaxAttempts: s.MaxAttempts,
		Backoff:         queuedapi.Backoff{Kind: s.Backoff.Kind, Initial: s.Backoff.Initial.String(), Max: s.Backoff.Max.String()},
		DeliveryTimeout: s.DeliveryTimeout.String(), Retention: s.Retention.String(), MaxMessageBytes: s.MaxMessageBytes,
	}
}

func toEntityStats(report queuedapi.StatsReport) []entities.Stats {
	result := make([]entities.Stats, 0, len(report.Stats))
	for _, s := range report.Stats {
		result = append(result, entities.Stats{
			QueueID: s.QueueID, Depth: s.Depth, InFlight: s.InFlight, OldestAgeSeconds: s.OldestAgeSeconds,
			DeliveredPerMinute: s.DeliveredPerMinute, FailedPerMinute: s.FailedPerMinute,
			DeadLetters: s.DeadLetters, NewestDead: toEntityDead(s.NewestDead),
		})
	}
	return result
}

func toEntityDead(dead []queuedapi.DeadLetter) []entities.DeadLetter {
	result := make([]entities.DeadLetter, 0, len(dead))
	for _, d := range dead {
		result = append(result, entities.DeadLetter(d))
	}
	return result
}

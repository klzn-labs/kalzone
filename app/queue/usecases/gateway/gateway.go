package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/usecases/credentials"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
)

type QueueSource interface {
	All(ctx context.Context) ([]entities.Queue, error)
	PendingCommands(ctx context.Context) ([]entities.Command, error)
	FinishCommand(ctx context.Context, id string, at time.Time) error
	AllCredentials(ctx context.Context) ([]entities.Credentials, error)
}

type ServiceLookup interface {
	FindServiceByID(ctx context.Context, id string) (queues.ServiceRef, error)
}

type PortLookup interface {
	Port(ctx context.Context, serviceID, environment string) (int, error)
}

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type DesiredQueue struct {
	ID              string
	Name            string
	Environment     string
	Version         int
	Settings        entities.Settings
	Paused          bool
	Target          string
	PublisherHashes []string
	SigningKey      string
	PreviousKey     string
}

type Desired struct {
	Queues   []DesiredQueue
	Commands []entities.Command
}

type GatewayUsecase struct {
	queues   QueueSource
	services ServiceLookup
	ports    PortLookup
	creds    Credentials
	stats    *StatsStore
}

func NewGatewayUsecase(queues QueueSource, services ServiceLookup, ports PortLookup, creds Credentials, stats *StatsStore) GatewayUsecase {
	return GatewayUsecase{queues: queues, services: services, ports: ports, creds: creds, stats: stats}
}

func (u GatewayUsecase) Desired(ctx context.Context) (Desired, error) {
	all, err := u.queues.All(ctx)
	if err != nil {
		return Desired{}, err
	}
	hashes, err := u.tokenHashes(ctx)
	if err != nil {
		return Desired{}, err
	}
	commands, err := u.queues.PendingCommands(ctx)
	return Desired{Queues: u.desiredQueues(ctx, all, hashes), Commands: commands}, err
}

func (u GatewayUsecase) desiredQueues(ctx context.Context, all []entities.Queue, hashes map[string]string) []DesiredQueue {
	result := make([]DesiredQueue, 0, len(all))
	for _, q := range all {
		d, ok, err := u.desiredQueue(ctx, q, hashes)
		if err != nil {
			log.Printf("queue %s/%s left out of the gateway's desired state: %v", q.Environment, q.Name, err)
		}
		if ok && err == nil {
			result = append(result, d)
		}
	}
	return result
}

func (u GatewayUsecase) desiredQueue(ctx context.Context, q entities.Queue, hashes map[string]string) (DesiredQueue, bool, error) {
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if errors.Is(err, apperror.NotFound("")) {
		return DesiredQueue{}, false, nil
	}
	if err != nil {
		return DesiredQueue{}, false, err
	}
	target, err := u.target(ctx, consumer, q)
	if err != nil {
		return DesiredQueue{}, false, err
	}
	keys, err := u.creds.Ensure(ctx, consumer.ID, q.Environment)
	return DesiredQueue{
		ID: q.ID, Name: q.Name, Environment: q.Environment, Version: q.Version, Settings: q.Settings, Paused: q.Paused,
		Target: target, PublisherHashes: publisherHashes(q, hashes), SigningKey: keys.SigningKey, PreviousKey: keys.PreviousKey,
	}, true, err
}

func (u GatewayUsecase) target(ctx context.Context, consumer queues.ServiceRef, q entities.Queue) (string, error) {
	port, err := u.ports.Port(ctx, consumer.ID, q.Environment)
	return fmt.Sprintf("http://%s-%s:%d%s", consumer.Slug, q.Environment, port, q.Settings.Path), err
}

func (u GatewayUsecase) tokenHashes(ctx context.Context) (map[string]string, error) {
	all, err := u.queues.AllCredentials(ctx)
	hashes := make(map[string]string, len(all))
	for _, c := range all {
		hashes[c.ServiceID+"|"+c.Environment] = c.TokenHash
	}
	return hashes, err
}

func publisherHashes(q entities.Queue, hashes map[string]string) []string {
	result := []string{}
	for _, id := range q.Publishers {
		if h, ok := hashes[id+"|"+q.Environment]; ok && !slices.Contains(result, h) {
			result = append(result, h)
		}
	}
	return result
}

func (u GatewayUsecase) Report(_ context.Context, stats []entities.Stats) error {
	u.stats.Put(stats, clock.Now().UTC())
	return nil
}

func (u GatewayUsecase) Done(ctx context.Context, commandID string) error {
	return u.queues.FinishCommand(ctx, commandID, clock.Now().UTC())
}

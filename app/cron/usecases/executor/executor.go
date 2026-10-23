package executor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

type Source interface {
	All(ctx context.Context) ([]entities.Cron, error)
	PendingCommands(ctx context.Context) ([]entities.Command, error)
	FinishCommand(ctx context.Context, id string, at time.Time) error
	RecordRun(ctx context.Context, run entities.Run) error
}

type ServiceLookup interface {
	FindServiceByID(ctx context.Context, id string) (crons.ServiceRef, error)
}

type PortLookup interface {
	Port(ctx context.Context, serviceID, environment string) (int, error)
}

type Keys struct {
	SigningKey  string
	PreviousKey string
}

type Credentials interface {
	Keys(ctx context.Context, serviceID, environment string) (Keys, error)
}

type DesiredCron struct {
	ID          string
	Name        string
	Environment string
	Service     string
	Schedule    string
	Target      string
	Payload     []byte
	Timeout     time.Duration
	Paused      bool
	SigningKey  string
	PreviousKey string
}

type Desired struct {
	Crons    []DesiredCron
	Commands []entities.Command
}

type ExecutorUsecase struct {
	crons    Source
	services ServiceLookup
	ports    PortLookup
	creds    Credentials
}

func NewExecutorUsecase(crons Source, services ServiceLookup, ports PortLookup, creds Credentials) ExecutorUsecase {
	return ExecutorUsecase{crons: crons, services: services, ports: ports, creds: creds}
}

func (u ExecutorUsecase) Desired(ctx context.Context) (Desired, error) {
	all, err := u.crons.All(ctx)
	if err != nil {
		return Desired{}, err
	}
	pending, err := u.crons.PendingCommands(ctx)
	if err != nil {
		return Desired{}, err
	}
	desired := u.desiredCrons(ctx, all)
	return Desired{Crons: desired, Commands: commandsFor(desired, pending)}, nil
}

func (u ExecutorUsecase) desiredCrons(ctx context.Context, all []entities.Cron) []DesiredCron {
	result := make([]DesiredCron, 0, len(all))
	for _, c := range all {
		d, ok, err := u.desiredCron(ctx, c)
		if err != nil {
			log.Printf("cron %s/%s left out of the gateway's desired state: %v", c.Environment, c.Name, err)
		}
		if ok && err == nil {
			result = append(result, d)
		}
	}
	return result
}

func (u ExecutorUsecase) desiredCron(ctx context.Context, c entities.Cron) (DesiredCron, bool, error) {
	service, err := u.services.FindServiceByID(ctx, c.ServiceID)
	if errors.Is(err, apperror.NotFound("")) || service.Decommissioning {
		return DesiredCron{}, false, nil
	}
	if err != nil {
		return DesiredCron{}, false, err
	}
	port, err := u.ports.Port(ctx, service.ID, c.Environment)
	if err != nil {
		return DesiredCron{}, false, err
	}
	keys, err := u.creds.Keys(ctx, service.ID, c.Environment)
	return DesiredCron{
		ID: c.ID, Name: c.Name, Environment: c.Environment, Service: service.Slug, Schedule: c.Schedule,
		Target: target(service.Slug, c.Environment, port, c.Path), Payload: c.Body(), Timeout: c.Timeout, Paused: c.Paused,
		SigningKey: keys.SigningKey, PreviousKey: keys.PreviousKey,
	}, true, err
}

func target(slug, environment string, port int, path string) string {
	return fmt.Sprintf("http://%s-%s:%d%s", slug, environment, port, path)
}

func commandsFor(desired []DesiredCron, pending []entities.Command) []entities.Command {
	known := make(map[string]bool, len(desired))
	for _, d := range desired {
		known[d.ID] = true
	}
	result := []entities.Command{}
	for _, c := range pending {
		if known[c.CronID] {
			result = append(result, c)
		}
	}
	return result
}

func (u ExecutorUsecase) Record(ctx context.Context, runs []entities.Run) error {
	for _, run := range runs {
		if err := u.crons.RecordRun(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (u ExecutorUsecase) Done(ctx context.Context, commandID string) error {
	return u.crons.FinishCommand(ctx, commandID, clock.Now().UTC())
}

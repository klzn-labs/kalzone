package registry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/klzn-labs/kalzone/internal/queued/backend"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

var errNotRunning = errors.New("the queue is desired but not running yet")

func splitDeletes(commands []queuedapi.Command) ([]queuedapi.Command, []queuedapi.Command) {
	deletes, others := []queuedapi.Command{}, []queuedapi.Command{}
	for _, c := range commands {
		if c.Kind == "delete" {
			deletes = append(deletes, c)
			continue
		}
		others = append(others, c)
	}
	return deletes, others
}

func (r *Registry) runCommands(ctx context.Context, commands []queuedapi.Command, desired map[string]bool) ([]string, []error) {
	done, errs := []string{}, []error{}
	for _, c := range commands {
		if err := r.run(ctx, c, desired); err != nil {
			errs = append(errs, fmt.Errorf("command %s (%s): %w", c.ID, c.Kind, err))
			continue
		}
		done = append(done, c.ID)
	}
	return done, errs
}

func (r *Registry) run(ctx context.Context, c queuedapi.Command, desired map[string]bool) error {
	if c.Kind == "delete" {
		return r.delete(ctx, c)
	}
	e, ok := r.entry(c.QueueID)
	if ok {
		return r.runOn(ctx, e.queue, c)
	}
	if desired[c.QueueID] {
		return errNotRunning
	}
	log.Printf("command %s skipped: queue %s no longer exists", c.ID, c.QueueID)
	return nil
}

func (r *Registry) delete(ctx context.Context, c queuedapi.Command) error {
	target := deletedQueue(c.Argument)
	for _, q := range r.Queues() {
		if q.ID != c.QueueID && q.Environment == target.Environment && q.Name == target.Name {
			log.Printf("command %s keeps the streams of %s: queue %s uses them", c.ID, c.Argument, q.ID)
			return nil
		}
	}
	return r.backend.Remove(ctx, target)
}

func (r *Registry) runOn(ctx context.Context, q model.Queue, c queuedapi.Command) error {
	switch c.Kind {
	case "replay":
		if err := r.backend.Replay(ctx, q, c.Argument); !errors.Is(err, backend.ErrDeadNotFound) {
			return err
		}
		return nil
	case "replay_all":
		_, err := r.backend.ReplayAll(ctx, q)
		return err
	case "purge_dead":
		return r.backend.PurgeDead(ctx, q)
	}
	log.Printf("command %s has unknown kind %q", c.ID, c.Kind)
	return nil
}

func deletedQueue(argument string) model.Queue {
	environment, name, _ := strings.Cut(argument, "/")
	return model.Queue{Environment: environment, Name: name}
}

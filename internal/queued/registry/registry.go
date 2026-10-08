package registry

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/adrianozp/platform/internal/queued/backend"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/pkg/queuedapi"
)

type LookupResult int

const (
	Found LookupResult = iota
	Forbidden
	NotFound
	Unauthorized
)

type Dispatcher interface {
	Start(ctx context.Context)
	Update(q model.Queue)
	Stop()
}

type Factory func(q model.Queue) Dispatcher

type entry struct {
	queue      model.Queue
	dispatcher Dispatcher
}

type Registry struct {
	ctx     context.Context
	backend backend.Backend
	factory Factory
	mu      sync.RWMutex
	entries map[string]entry
}

func New(ctx context.Context, b backend.Backend, factory Factory) *Registry {
	return &Registry{ctx: ctx, backend: b, factory: factory, entries: map[string]entry{}}
}

func (r *Registry) Apply(ctx context.Context, desired queuedapi.Desired) ([]string, []error) {
	deletes, others := splitDeletes(desired.Commands)
	doneDeletes, deleteErrs := r.runCommands(ctx, deletes, nil)
	errs := r.applyQueues(ctx, desired.Queues)
	done, commandErrs := r.runCommands(ctx, others, desiredIDs(desired.Queues))
	return append(doneDeletes, done...), append(append(deleteErrs, errs...), commandErrs...)
}

func (r *Registry) applyQueues(ctx context.Context, queues []queuedapi.Queue) []error {
	errs := []error{}
	seen := map[string]bool{}
	for _, raw := range queues {
		seen[raw.ID] = true
		q, err := model.FromAPI(raw)
		if err == nil {
			err = r.upsert(ctx, q)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("queue %s/%s: %w", raw.Environment, raw.Name, err))
		}
	}
	r.stopMissing(seen)
	return errs
}

func desiredIDs(queues []queuedapi.Queue) map[string]bool {
	ids := make(map[string]bool, len(queues))
	for _, q := range queues {
		ids[q.ID] = true
	}
	return ids
}

func (r *Registry) upsert(ctx context.Context, q model.Queue) error {
	current, known := r.entry(q.ID)
	if !known || current.queue.Version != q.Version {
		if err := r.backend.Ensure(ctx, q); err != nil {
			return err
		}
	}
	if known {
		current.dispatcher.Update(q)
		r.store(entry{queue: q, dispatcher: current.dispatcher})
		return nil
	}
	d := r.factory(q)
	d.Start(r.ctx)
	r.store(entry{queue: q, dispatcher: d})
	return nil
}

func (r *Registry) stopMissing(seen map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, e := range r.entries {
		if !seen[id] {
			e.dispatcher.Stop()
			delete(r.entries, id)
		}
	}
}

func (r *Registry) entry(id string) (entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	return e, ok
}

func (r *Registry) store(e entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.queue.ID] = e
}

func (r *Registry) Queues() []model.Queue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Queue, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, e.queue)
	}
	return result
}

func (r *Registry) Lookup(name, tokenHash string) (model.Queue, LookupResult) {
	queues := r.Queues()
	if !knownToken(queues, tokenHash) {
		return model.Queue{}, Unauthorized
	}
	result := NotFound
	for _, q := range queues {
		if q.Name != name {
			continue
		}
		if slices.Contains(q.PublisherHashes, tokenHash) {
			return q, Found
		}
		result = Forbidden
	}
	return model.Queue{}, result
}

func knownToken(queues []model.Queue, tokenHash string) bool {
	for _, q := range queues {
		if slices.Contains(q.PublisherHashes, tokenHash) {
			return true
		}
	}
	return false
}

func (r *Registry) StopAll() {
	r.stopMissing(map[string]bool{})
}

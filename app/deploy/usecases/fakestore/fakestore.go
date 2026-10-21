package fakestore

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type Store struct {
	ByID      map[string]entities.Deployment
	Lines     map[string][]string
	Seen      map[string]entities.Agent
	Runtimes  map[string]entities.RuntimeStatus
	Platforms map[string]entities.PlatformComponent
}

func New() *Store {
	return &Store{
		ByID: map[string]entities.Deployment{}, Lines: map[string][]string{}, Seen: map[string]entities.Agent{},
		Runtimes: map[string]entities.RuntimeStatus{}, Platforms: map[string]entities.PlatformComponent{},
	}
}

func (s *Store) Create(_ context.Context, d entities.Deployment) (entities.Deployment, error) {
	d.Number = len(s.matching(func(o entities.Deployment) bool { return o.ServiceID == d.ServiceID && o.Environment == d.Environment })) + 1
	d.CreatedAt = d.CreatedAt.Add(time.Duration(len(s.ByID)) * time.Millisecond)
	s.ByID[d.ID] = d
	return d, nil
}

func (s *Store) ScheduleAll(ctx context.Context, ds []entities.Deployment) ([]entities.Deployment, error) {
	created := make([]entities.Deployment, 0, len(ds))
	for _, d := range ds {
		s.supersede(d)
		next, _ := s.Create(ctx, d)
		created = append(created, next)
	}
	return created, nil
}

func (s *Store) supersede(d entities.Deployment) {
	for _, old := range s.matching(func(o entities.Deployment) bool {
		return o.ServiceID == d.ServiceID && o.Environment == d.Environment && o.Status == entities.StatusPending
	}) {
		old.Status = entities.StatusCancelled
		s.ByID[old.ID] = old
	}
}

func (s *Store) Transition(_ context.Context, d entities.Deployment, from entities.Status) error {
	if s.ByID[d.ID].Status != from {
		return apperror.Conflict("changed")
	}
	s.ByID[d.ID] = d
	return nil
}

func (s *Store) FindByID(_ context.Context, id string) (entities.Deployment, error) {
	d, ok := s.ByID[id]
	if !ok {
		return entities.Deployment{}, apperror.NotFound("deployment not found").WithHint("list deployments with GET /api/v1/deployments?service=<slug>&env=<env>")
	}
	return d, nil
}

func (s *Store) History(_ context.Context, serviceID, env string, before, limit int) ([]entities.Deployment, error) {
	found := s.matching(func(d entities.Deployment) bool {
		return d.ServiceID == serviceID && d.Environment == env && (before == 0 || d.Number < before)
	})
	slices.Reverse(found)
	return found[:min(limit, len(found))], nil
}

func (s *Store) Active(_ context.Context, serviceID, env string) ([]entities.Deployment, error) {
	return s.matching(func(d entities.Deployment) bool {
		return d.ServiceID == serviceID && d.Environment == env && d.Active()
	}), nil
}

func (s *Store) LatestSucceeded(_ context.Context, serviceID, env string) (entities.Deployment, bool, error) {
	found := s.matching(func(d entities.Deployment) bool {
		return d.ServiceID == serviceID && d.Environment == env && d.Status == entities.StatusSucceeded
	})
	if len(found) == 0 {
		return entities.Deployment{}, false, nil
	}
	return found[len(found)-1], true, nil
}

func (s *Store) AllLive(ctx context.Context) ([]entities.Deployment, error) {
	seen := map[string]entities.Deployment{}
	for _, d := range s.matching(func(d entities.Deployment) bool { return d.Status == entities.StatusSucceeded }) {
		seen[d.ServiceID+"/"+d.Environment] = d
	}
	result := []entities.Deployment{}
	for _, d := range seen {
		result = append(result, d)
	}
	return result, nil
}

func (s *Store) Environments(_ context.Context, serviceID string) ([]string, error) {
	environments := []string{}
	for _, d := range s.ByID {
		if d.ServiceID == serviceID && !slices.Contains(environments, d.Environment) {
			environments = append(environments, d.Environment)
		}
	}
	sort.Strings(environments)
	return environments, nil
}

func (s *Store) OldestPending(context.Context) (entities.Deployment, bool, error) {
	found := s.matching(func(d entities.Deployment) bool { return d.Status == entities.StatusPending })
	if len(found) == 0 {
		return entities.Deployment{}, false, nil
	}
	return found[0], true, nil
}

func (s *Store) StaleInProgress(_ context.Context, before time.Time) ([]entities.Deployment, error) {
	return s.matching(func(d entities.Deployment) bool {
		return d.Status == entities.StatusInProgress && d.StartedAt != nil && d.StartedAt.Before(before)
	}), nil
}

func (s *Store) AppendEvents(_ context.Context, id string, _ time.Time, lines []string) error {
	s.Lines[id] = append(s.Lines[id], lines...)
	return nil
}

func (s *Store) Events(_ context.Context, id string, limit int) ([]entities.Event, error) {
	all := s.events(id)
	return all[max(0, len(all)-limit):], nil
}

func (s *Store) EventsAfter(_ context.Context, id string, after int64, limit int) ([]entities.Event, error) {
	all := s.events(id)
	start := min(int(after), len(all))
	return all[start:min(start+limit, len(all))], nil
}

func (s *Store) CountEvents(_ context.Context, id string) (int, error) {
	return len(s.Lines[id]), nil
}

func (s *Store) events(id string) []entities.Event {
	result := []entities.Event{}
	for i, line := range s.Lines[id] {
		result = append(result, entities.Event{ID: int64(i + 1), DeploymentID: id, Message: line})
	}
	return result
}

func (s *Store) TouchAgent(_ context.Context, a entities.Agent) error {
	s.Seen[a.Name] = a
	return nil
}

func (s *Store) Agents(context.Context) ([]entities.Agent, error) { return nil, nil }

func (s *Store) FinishedSince(_ context.Context, environment string, since time.Time) ([]entities.Deployment, error) {
	return s.matching(func(d entities.Deployment) bool {
		finished := d.Status == entities.StatusSucceeded || d.Status == entities.StatusFailed
		return finished && d.Environment == environment && d.FinishedAt != nil && !d.FinishedAt.Before(since)
	}), nil
}

func (s *Store) matching(keep func(entities.Deployment) bool) []entities.Deployment {
	result := []entities.Deployment{}
	for _, d := range s.ByID {
		if keep(d) {
			result = append(result, d)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

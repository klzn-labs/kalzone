package fakestore

import (
	"context"
	"sort"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/apperror"
)

type Store struct {
	ByID  map[string]entities.Deployment
	Lines map[string][]string
	Seen  map[string]entities.Agent
}

func New() *Store {
	return &Store{ByID: map[string]entities.Deployment{}, Lines: map[string][]string{}, Seen: map[string]entities.Agent{}}
}

func (s *Store) Create(_ context.Context, d entities.Deployment) (entities.Deployment, error) {
	d.Number = len(s.matching(func(o entities.Deployment) bool { return o.ServiceID == d.ServiceID && o.Environment == d.Environment })) + 1
	d.CreatedAt = d.CreatedAt.Add(time.Duration(len(s.ByID)) * time.Millisecond)
	s.ByID[d.ID] = d
	return d, nil
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
		return entities.Deployment{}, apperror.NotFound("deployment not found")
	}
	return d, nil
}

func (s *Store) History(_ context.Context, serviceID, env string, _, _ int) ([]entities.Deployment, error) {
	return s.matching(func(d entities.Deployment) bool { return d.ServiceID == serviceID && d.Environment == env }), nil
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

func (s *Store) Events(_ context.Context, id string, _ int) ([]entities.Event, error) {
	result := []entities.Event{}
	for _, line := range s.Lines[id] {
		result = append(result, entities.Event{DeploymentID: id, Message: line})
	}
	return result, nil
}

func (s *Store) TouchAgent(_ context.Context, a entities.Agent) error {
	s.Seen[a.Name] = a
	return nil
}

func (s *Store) Agents(context.Context) ([]entities.Agent, error) { return nil, nil }

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

package fakestore

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
)

func (s *Store) SaveRuntime(_ context.Context, statuses []entities.RuntimeStatus) error {
	for _, status := range statuses {
		s.Runtimes[status.ServiceID+"/"+status.Environment] = status
	}
	return nil
}

func (s *Store) Runtime(_ context.Context, serviceID, environment string) (entities.RuntimeStatus, bool, error) {
	status, found := s.Runtimes[serviceID+"/"+environment]
	return status, found, nil
}

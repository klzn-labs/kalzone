package query_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/audit/usecases/query"
	"github.com/stretchr/testify/require"
)

type store struct{ limits []int }

func (s *store) List(_ context.Context, f entities.Filter) ([]entities.Event, error) {
	s.limits = append(s.limits, f.Limit)
	return nil, nil
}

func Test_List_ClampsTheLimit(t *testing.T) {
	s := &store{}
	usecase := query.NewQueryUsecase(s)

	for _, limit := range []int{0, 10, 1000} {
		_, err := usecase.List(context.Background(), entities.Filter{Limit: limit})
		require.NoError(t, err)
	}

	require.Equal(t, []int{50, 10, 200}, s.limits)
}

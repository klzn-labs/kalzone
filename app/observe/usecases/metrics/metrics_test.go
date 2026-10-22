package metrics_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/metrics"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	ana = actor.Actor{UserID: "u-ana", Teams: []string{"t1"}}
)

type services struct{}

func (services) FindService(_ context.Context, slug string) (entities.LogSource, error) {
	if slug != "places-api" {
		return entities.LogSource{}, apperror.NotFound("service not found")
	}
	return entities.LogSource{ID: "s1", Slug: slug, OwnerTeamID: "t1"}, nil
}

type querier struct {
	mu      sync.Mutex
	queries []string
	steps   []time.Duration
	starts  []time.Time
}

func (q *querier) Range(_ context.Context, _, query string, start, _ time.Time, step time.Duration) ([]entities.Sample, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queries, q.steps, q.starts = append(q.queries, query), append(q.steps, step), append(q.starts, start)
	if strings.Contains(query, "container_memory") {
		return nil, errors.New("prometheus is down")
	}
	return []entities.Sample{{At: now, Value: 1}}, nil
}

func Test_Read_QueriesEverySignalOverTheWindow(t *testing.T) {
	clock.WithTime(now)
	q := &querier{}

	m, err := metrics.NewMetricsUsecase(services{}, q, "prometheus").Read(ctx, ana, "places-api", "dev", entities.WindowNamed("1h"))

	require.NoError(t, err)
	require.Len(t, q.queries, 4)
	require.Equal(t, []time.Duration{time.Minute, time.Minute, time.Minute, time.Minute}, q.steps)
	require.Equal(t, now.Add(-time.Hour), q.starts[0])
	require.Len(t, m.Series[entities.SignalLatency], 1)
	require.Empty(t, m.Series[entities.SignalMemory])
}

func Test_Read_HidesServicesOfOtherTeams(t *testing.T) {
	q := &querier{}
	outsider := actor.Actor{UserID: "u-zed", Teams: []string{"t9"}}

	_, err := metrics.NewMetricsUsecase(services{}, q, "prometheus").Read(ctx, outsider, "places-api", "dev", entities.WindowNamed("6h"))

	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Empty(t, q.queries)
}

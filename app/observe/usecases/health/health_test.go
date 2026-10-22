package health_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/health"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type querier struct {
	mu      sync.Mutex
	values  map[string]float64
	failing string
	asked   []string
}

func (q *querier) match(query string) (float64, bool) {
	for part, v := range q.values {
		if strings.Contains(query, part) {
			return v, true
		}
	}
	return 0, false
}

func (q *querier) Instant(_ context.Context, _, query string) (float64, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked = append(q.asked, query)
	if q.failing != "" && strings.Contains(query, q.failing) {
		return 0, false, errors.New("down")
	}
	v, ok := q.match(query)
	return v, ok, nil
}

func (q *querier) Range(_ context.Context, _, query string, _, end time.Time, step time.Duration) ([]entities.Sample, error) {
	v, ok := q.match(query)
	if !ok {
		return nil, nil
	}
	return []entities.Sample{{At: end.Add(-2 * step), Value: v * 2}, {At: end.Add(-step), Value: v}, {At: end, Value: v}}, nil
}

func usecase(q *querier) health.HealthUsecase {
	cfg := config.Config{Grafana: config.Grafana{PrometheusUID: "prometheus"}, Health: config.Health{ResponseMS: 300, CPUPercent: 70, DBCPUPercent: 70}}
	return health.NewHealthUsecase(q, cfg)
}

func byMetric(h entities.Health) map[entities.Metric]entities.Reading {
	result := map[entities.Metric]entities.Reading{}
	for _, r := range h.Readings {
		result[r.Metric] = r
	}
	return result
}

func Test_NowReadsEachMetricAgainstTheDefaultOrTheServiceLimit(t *testing.T) {
	q := &querier{values: map[string]float64{"traefik": 410, `name=~"legendsgrid`: 12, "plateful-mysql-1": 46}}
	target := health.Target{Slug: "legendsgrid", CPUMillis: 500, Database: true, Limits: catalog.Limits{ResponseMS: 500}}

	readings := byMetric(usecase(q).Now(ctx, target))

	require.Equal(t, 500, readings[entities.MetricResponse].Limit)
	require.False(t, readings[entities.MetricResponse].OverNow())
	require.Equal(t, 70, readings[entities.MetricCPU].Limit)
	require.Equal(t, 46.0, readings[entities.MetricDBCPU].Now)
	require.Contains(t, q.asked[1], "* 100000 / 500")
}

func Test_NowSkipsMetricsTheServiceCannotHave(t *testing.T) {
	q := &querier{values: map[string]float64{"traefik": 120}}

	h := usecase(q).Now(ctx, health.Target{Slug: "worker"})

	require.Len(t, h.Readings, 1)
	require.Equal(t, entities.MetricResponse, h.Readings[0].Metric)
}

func Test_AFailedQueryIsUnknownAndNeverOver(t *testing.T) {
	q := &querier{values: map[string]float64{"traefik": 900}, failing: "traefik"}

	h := usecase(q).Now(ctx, health.Target{Slug: "api"})

	require.False(t, h.Readings[0].Known)
	require.Empty(t, h.Over())
}

func Test_DayKeepsTheSeriesAndItsPeak(t *testing.T) {
	clock.WithTime(time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	q := &querier{values: map[string]float64{"plateful-mysql-1": 46}}

	h := usecase(q).Day(ctx, health.Target{Slug: "legendsgrid", Database: true})
	db := byMetric(h)[entities.MetricDBCPU]

	require.Len(t, db.Series, 3)
	require.Equal(t, 92.0, db.Peak)
	require.True(t, db.WentOver())
	require.False(t, db.OverNow())
}

func Test_NowAllAnswersForEveryService(t *testing.T) {
	q := &querier{values: map[string]float64{`name=~"api`: 80, "traefik": 100}}

	all := usecase(q).NowAll(ctx, []health.Target{{Slug: "api", CPUMillis: 250}, {Slug: "web"}})

	require.Len(t, all, 2)
	require.Len(t, all["api"].Over(), 1)
	require.Empty(t, all["web"].Over())
}

func Test_AMissingDefaultFallsBackToTheBuiltInLimit(t *testing.T) {
	q := &querier{values: map[string]float64{"traefik": 120}}

	h := health.NewHealthUsecase(q, config.Config{}).Now(ctx, health.Target{Slug: "api"})

	require.Equal(t, 300, h.Readings[0].Limit)
	require.Empty(t, h.Over())
}

func Test_ResponseLimitIsTheServiceOverrideOrTheDefault(t *testing.T) {
	u := health.NewHealthUsecase(&querier{}, config.Config{})

	require.Equal(t, 300, u.ResponseLimit(catalog.Limits{}))
	require.Equal(t, 800, u.ResponseLimit(catalog.Limits{ResponseMS: 800}))
}

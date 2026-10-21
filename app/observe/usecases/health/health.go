package health

import (
	"context"
	"sync"
	"time"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var builtIn = config.Health{ResponseMS: 300, CPUPercent: 70, DBCPUPercent: 70}

const (
	queryTimeout = 3 * time.Second
	day          = 24 * time.Hour
	step         = 5 * time.Minute
)

type Querier interface {
	Instant(ctx context.Context, uid, query string) (float64, bool, error)
	Range(ctx context.Context, uid, query string, start, end time.Time, step time.Duration) ([]entities.Sample, error)
}

type Target struct {
	Slug      string
	CPUMillis int
	Database  bool
	Limits    catalog.Limits
}

type probe struct {
	metric entities.Metric
	query  string
	limit  int
}

type HealthUsecase struct {
	querier  Querier
	uid      string
	defaults config.Health
}

func NewHealthUsecase(querier Querier, cfg config.Config) HealthUsecase {
	defaults := config.Health{
		ResponseMS:   or(cfg.Health.ResponseMS, builtIn.ResponseMS),
		CPUPercent:   or(cfg.Health.CPUPercent, builtIn.CPUPercent),
		DBCPUPercent: or(cfg.Health.DBCPUPercent, builtIn.DBCPUPercent),
	}
	return HealthUsecase{querier: querier, uid: cfg.Grafana.PrometheusUID, defaults: defaults}
}

func (u HealthUsecase) Now(ctx context.Context, t Target) entities.Health {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	h := entities.Health{}
	for _, p := range u.probes(t) {
		value, found, err := u.querier.Instant(ctx, u.uid, p.query)
		h.Readings = append(h.Readings, entities.Reading{Metric: p.metric, Limit: p.limit, Now: value, Peak: value, Known: found && err == nil})
	}
	return h
}

func (u HealthUsecase) Day(ctx context.Context, t Target) entities.Health {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	end := clock.Now()
	h := entities.Health{}
	for _, p := range u.probes(t) {
		samples, _ := u.querier.Range(ctx, u.uid, p.query, end.Add(-day), end, step)
		h.Readings = append(h.Readings, entities.ReadingOf(p.metric, p.limit, samples, step))
	}
	return h
}

func (u HealthUsecase) NowAll(ctx context.Context, targets []Target) map[string]entities.Health {
	var mu sync.Mutex
	var wg sync.WaitGroup
	result := make(map[string]entities.Health, len(targets))
	for _, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := u.Now(ctx, t)
			mu.Lock()
			result[t.Slug] = h
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

func (u HealthUsecase) probes(t Target) []probe {
	probes := []probe{{entities.MetricResponse, entities.ResponseQuery(t.Slug), or(t.Limits.ResponseMS, u.defaults.ResponseMS)}}
	if t.CPUMillis > 0 {
		probes = append(probes, probe{entities.MetricCPU, entities.CPUQuery(t.Slug, t.CPUMillis), or(t.Limits.CPUPercent, u.defaults.CPUPercent)})
	}
	if t.Database {
		probes = append(probes, probe{entities.MetricDBCPU, entities.DBCPUQuery(), or(t.Limits.DBCPUPercent, u.defaults.DBCPUPercent)})
	}
	return probes
}

func or(override, fallback int) int {
	if override > 0 {
		return override
	}
	return fallback
}

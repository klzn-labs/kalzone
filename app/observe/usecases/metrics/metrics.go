package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const queryTimeout = 4 * time.Second

var errNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")

type Services interface {
	FindService(ctx context.Context, slug string) (entities.LogSource, error)
}

type Querier interface {
	Range(ctx context.Context, uid, query string, start, end time.Time, step time.Duration) ([]entities.Sample, error)
}

type MetricsUsecase struct {
	services Services
	querier  Querier
	uid      string
}

func NewMetricsUsecase(services Services, querier Querier, uid string) MetricsUsecase {
	return MetricsUsecase{services: services, querier: querier, uid: uid}
}

func (u MetricsUsecase) Read(ctx context.Context, a actor.Actor, serviceSlug, environment string, w entities.Window) (entities.Metrics, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.Metrics{}, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Metrics{}, errNotVisible
	}
	return u.query(ctx, service.Slug, environment, w), nil
}

func (u MetricsUsecase) query(ctx context.Context, slug, environment string, w entities.Window) entities.Metrics {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	m := entities.Metrics{Window: w, End: clock.Now(), Series: map[entities.Signal][]entities.Sample{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, s := range entities.Signals {
		wg.Go(func() {
			samples, _ := u.querier.Range(ctx, u.uid, entities.SignalQuery(s, slug, environment, w.Step), m.Start(), m.End, w.Step)
			mu.Lock()
			m.Series[s] = samples
			mu.Unlock()
		})
	}
	wg.Wait()
	return m
}

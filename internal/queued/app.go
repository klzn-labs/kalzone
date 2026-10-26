package queued

import (
	"context"
	"net/http"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/backend/jetstream"
	"github.com/klzn-labs/kalzone/internal/queued/controlplane"
	"github.com/klzn-labs/kalzone/internal/queued/cron"
	"github.com/klzn-labs/kalzone/internal/queued/dispatch"
	"github.com/klzn-labs/kalzone/internal/queued/governor"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/internal/queued/publish"
	"github.com/klzn-labs/kalzone/internal/queued/registry"
	"github.com/klzn-labs/kalzone/internal/queued/runner"
	"github.com/klzn-labs/kalzone/internal/queued/stats"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const outboxLimit = 1000

type App struct {
	cfg       Config
	driver    *jetstream.Driver
	registry  *registry.Registry
	scheduler *cron.Scheduler
	outbox    *cron.Outbox
	collector *stats.Collector
	governor  *governor.Governor
	throttle  model.Throttle
	metrics   *prometheus.Registry
	cancel    context.CancelFunc
	ctx       context.Context
}

func Build(cfg Config, conn *nats.Conn) (*App, error) {
	driver, err := jetstream.New(conn)
	if err != nil {
		return nil, err
	}
	app := &App{cfg: cfg, driver: driver, metrics: prometheus.NewRegistry()}
	if app.governor, err = cfg.Adaptive.build(app.metrics); err != nil {
		return nil, err
	}
	app.throttle = throttleOf(app.governor)
	ctx, cancel := context.WithCancel(context.Background())
	app.ctx, app.cancel = ctx, cancel
	app.registry = registry.New(ctx, driver, app.dispatcher)
	app.outbox = cron.NewOutbox(outboxLimit)
	app.scheduler = cron.New(ctx, cron.Options{Client: &http.Client{}, Clock: cron.RealClock{}, Backoff: cron.DefaultBackoff, Outbox: app.outbox})
	app.collector = stats.New(app.registry, driver, app.throttle, app.metrics, time.Now)
	return app, nil
}

func (a *App) dispatcher(q model.Queue) registry.Dispatcher {
	return dispatch.New(a.driver, &http.Client{}, q, a.collector, a.throttle)
}

func (a *App) Handler() http.Handler {
	return publish.New(a.registry, a.driver).Handler()
}

func (a *App) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(a.metrics, promhttp.HandlerOpts{})
}

func (a *App) Run(ctx context.Context) {
	client := controlplane.New(a.cfg.PlatformURL, a.cfg.Token, &http.Client{Timeout: 10 * time.Second})
	queuesAndCrons := applier{queues: a.registry, crons: a.scheduler}
	if a.governor != nil {
		go a.governor.Run(ctx)
	}
	runner.New(client, queuesAndCrons, a.collector, a.outbox, a.cfg.Poll, a.cfg.StatsEvery).Run(ctx)
}

func (a *App) Stop() {
	a.cancel()
	a.registry.StopAll()
	a.scheduler.StopAll()
}

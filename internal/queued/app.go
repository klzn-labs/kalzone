package queued

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/internal/queued/backend/jetstream"
	"github.com/adrianozp/platform/internal/queued/controlplane"
	"github.com/adrianozp/platform/internal/queued/dispatch"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/internal/queued/publish"
	"github.com/adrianozp/platform/internal/queued/registry"
	"github.com/adrianozp/platform/internal/queued/runner"
	"github.com/adrianozp/platform/internal/queued/stats"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type App struct {
	cfg       Config
	driver    *jetstream.Driver
	registry  *registry.Registry
	collector *stats.Collector
	metrics   *prometheus.Registry
	cancel    context.CancelFunc
	ctx       context.Context
}

func Build(cfg Config, conn *nats.Conn) (*App, error) {
	driver, err := jetstream.New(conn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{cfg: cfg, driver: driver, metrics: prometheus.NewRegistry(), cancel: cancel, ctx: ctx}
	app.registry = registry.New(ctx, driver, app.dispatcher)
	app.collector = stats.New(app.registry, driver, app.metrics, time.Now)
	return app, nil
}

func (a *App) dispatcher(q model.Queue) registry.Dispatcher {
	return dispatch.New(a.driver, &http.Client{}, q, a.collector)
}

func (a *App) Handler() http.Handler {
	return publish.New(a.registry, a.driver).Handler()
}

func (a *App) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(a.metrics, promhttp.HandlerOpts{})
}

func (a *App) Run(ctx context.Context) {
	client := controlplane.New(a.cfg.PlatformURL, a.cfg.Token, &http.Client{Timeout: 10 * time.Second})
	runner.New(client, a.registry, a.collector, a.cfg.Poll, a.cfg.StatsEvery).Run(ctx)
}

func (a *App) Stop() {
	a.cancel()
	a.registry.StopAll()
}

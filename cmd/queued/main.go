package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/adrianozp/platform/internal/queued"
	"github.com/nats-io/nats.go"
)

var Version = "dev"

func main() {
	cfg, err := queued.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	conn, err := connect(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Drain()
	app, err := queued.Build(cfg, conn)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("queued %s polling %s", Version, cfg.PlatformURL)
	serve(ctx, app, cfg)
}

func connect(cfg queued.Config) (*nats.Conn, error) {
	return nats.Connect(cfg.NATSURL, nats.UserInfo(cfg.NATSUser, cfg.NATSPassword), nats.Name("queued"),
		nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1), nats.ReconnectWait(2*time.Second))
}

func serve(ctx context.Context, app *queued.App, cfg queued.Config) {
	public := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	metrics := &http.Server{Addr: cfg.MetricsAddr, Handler: app.MetricsHandler(), ReadHeaderTimeout: 10 * time.Second}
	for _, s := range []*http.Server{public, metrics} {
		go listen(s)
	}
	app.Run(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	public.Shutdown(shutdown)
	metrics.Shutdown(shutdown)
	app.Stop()
}

func listen(s *http.Server) {
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

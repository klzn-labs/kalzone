package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/adrianozp/platform/internal/agent"
	"github.com/adrianozp/platform/internal/agent/deployctl"
)

var Version = "dev"

func main() {
	cfg, err := agent.LoadConfig(Version)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("agent %s %s polling %s", cfg.Name, cfg.Version, cfg.PlatformURL)
	agent.New(cfg, agent.NewClient(cfg), deployctl.New(cfg)).Run(ctx)
}

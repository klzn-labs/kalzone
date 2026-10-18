package agent

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

func (a Agent) StatusOnce(ctx context.Context) int {
	desired, err := a.plane.Desired(ctx)
	if err != nil {
		log.Printf("desired state: %v", err)
		return 0
	}
	snapshots := make([]agentapi.Snapshot, 0, len(desired))
	for _, d := range desired {
		snapshots = append(snapshots, a.snapshot(ctx, d))
	}
	if err := a.plane.Status(ctx, snapshots); err != nil {
		log.Printf("report status: %v", err)
		return 0
	}
	return len(snapshots)
}

func (a Agent) snapshot(ctx context.Context, d agentapi.Desired) agentapi.Snapshot {
	observed, err := a.runtime.Observe(ctx, d.Name)
	if err != nil {
		log.Printf("observe %s: %v", d.Name, err)
	}
	return agentapi.Snapshot{Service: d.Service, Environment: d.Environment, Container: observed.Container}
}

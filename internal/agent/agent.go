package agent

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/agentapi"
)

type ControlPlane interface {
	Reporter
	Claim(ctx context.Context) (agentapi.Workload, bool, error)
	Finish(ctx context.Context, deploymentID string, succeeded bool, message string) error
	Desired(ctx context.Context) ([]agentapi.Desired, error)
	Drift(ctx context.Context, d agentapi.Drift) error
}

type Agent struct {
	plane      ControlPlane
	runtime    Runtime
	poll       time.Duration
	driftEvery time.Duration
}

func New(cfg Config, plane ControlPlane, runtime Runtime) Agent {
	return Agent{plane: plane, runtime: runtime, poll: cfg.Poll, driftEvery: cfg.DriftEvery}
}

func (a Agent) Run(ctx context.Context) {
	poll, drift := time.NewTicker(a.poll), time.NewTicker(a.driftEvery)
	defer poll.Stop()
	defer drift.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			a.WorkOnce(ctx)
		case <-drift.C:
			a.ReconcileOnce(ctx)
		}
	}
}

func (a Agent) WorkOnce(ctx context.Context) bool {
	w, claimed, err := a.plane.Claim(ctx)
	if err != nil {
		log.Printf("claim: %v", err)
		return false
	}
	if !claimed {
		return false
	}
	a.run(ctx, w)
	return true
}

func (a Agent) run(ctx context.Context, w agentapi.Workload) {
	log.Printf("deploying %s #%d (%s)", w.Name, w.Number, w.Version)
	sink := newSink(ctx, a.plane, w.DeploymentID)
	err := a.runtime.Apply(ctx, w, sink)
	sink.Flush()
	succeeded, message := err == nil, "live"
	if err != nil {
		message = err.Error() + ": " + strings.Join(sink.Tail(), " | ")
	}
	if err := a.plane.Finish(ctx, w.DeploymentID, succeeded, message); err != nil {
		log.Printf("finish %s: %v", w.DeploymentID, err)
	}
}

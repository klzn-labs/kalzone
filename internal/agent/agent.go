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
	Finish(ctx context.Context, deploymentID string, succeeded bool, message string, failure *agentapi.Failure) error
	Desired(ctx context.Context) ([]agentapi.Desired, error)
	Drift(ctx context.Context, d agentapi.Drift) error
	Status(ctx context.Context, snapshots []agentapi.Snapshot) error
}

type Agent struct {
	plane       ControlPlane
	runtime     Runtime
	poll        time.Duration
	driftEvery  time.Duration
	statusEvery time.Duration
}

func New(cfg Config, plane ControlPlane, runtime Runtime) Agent {
	return Agent{plane: plane, runtime: runtime, poll: cfg.Poll, driftEvery: cfg.DriftEvery, statusEvery: cfg.StatusEvery}
}

func (a Agent) Run(ctx context.Context) {
	poll, drift, status := time.NewTicker(a.poll), time.NewTicker(a.driftEvery), time.NewTicker(a.statusEvery)
	defer poll.Stop()
	defer drift.Stop()
	defer status.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			a.WorkOnce(ctx)
		case <-drift.C:
			a.ReconcileOnce(ctx)
		case <-status.C:
			a.StatusOnce(ctx)
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
	if err == nil {
		a.finish(ctx, w, true, "live", nil)
		return
	}
	failure := a.evidence(ctx, w, err)
	a.finish(ctx, w, false, err.Error()+": "+strings.Join(sink.Tail(), " | "), &failure)
}

func (a Agent) finish(ctx context.Context, w agentapi.Workload, succeeded bool, message string, failure *agentapi.Failure) {
	if err := a.plane.Finish(context.WithoutCancel(ctx), w.DeploymentID, succeeded, message, failure); err != nil {
		log.Printf("finish %s: %v", w.DeploymentID, err)
	}
}

package agent

import (
	"context"
	"log"

	"github.com/adrianozp/platform/pkg/agentapi"
)

func (a Agent) ReconcileOnce(ctx context.Context) int {
	desired, err := a.plane.Desired(ctx)
	if err != nil {
		log.Printf("desired state: %v", err)
		return 0
	}
	reported := 0
	for _, d := range desired {
		if a.checkOne(ctx, d) {
			reported++
		}
	}
	return reported
}

func (a Agent) checkOne(ctx context.Context, d agentapi.Desired) bool {
	observed, err := a.runtime.Observe(ctx, d.Name)
	if err != nil {
		log.Printf("observe %s: %v", d.Name, err)
		return false
	}
	difference := describe(d, observed)
	if difference == "" {
		return false
	}
	drift := agentapi.Drift{Service: d.Service, Environment: d.Environment, Observed: difference}
	if err := a.plane.Drift(ctx, drift); err != nil {
		log.Printf("report drift of %s: %v", d.Name, err)
		return false
	}
	return true
}

func describe(d agentapi.Desired, o Observed) string {
	switch {
	case !o.Known:
		return d.Name + " has never been deployed on this host"
	case !o.Running:
		return d.Name + " is not running"
	case o.Image != d.ImageRef:
		return d.Name + " runs " + o.Image + " instead of " + d.ImageRef
	}
	return ""
}

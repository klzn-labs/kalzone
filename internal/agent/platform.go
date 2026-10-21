package agent

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

func (a Agent) PlatformOnce(ctx context.Context) int {
	reported := 0
	for _, name := range agentapi.PlatformComponents {
		if a.reportPlatform(ctx, name) {
			reported++
		}
	}
	return reported
}

func (a Agent) reportPlatform(ctx context.Context, name string) bool {
	report, found, err := a.runtime.Platform(ctx, name)
	if err != nil {
		log.Printf("observe platform %s: %v", name, err)
		return false
	}
	if !found {
		return false
	}
	if err := a.plane.Platform(ctx, name, report); err != nil {
		log.Printf("report platform %s: %v", name, err)
		return false
	}
	return true
}

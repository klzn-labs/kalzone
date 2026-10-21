package agents

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	platformHistory = 20
	reasonLength    = 500
)

var errUnknownComponent = apperror.NotFound("unknown platform component")

func (u AgentsUsecase) ReportPlatform(ctx context.Context, agent string, at time.Time, c entities.PlatformComponent) error {
	if !entities.IsPlatformComponent(c.Name) {
		return errUnknownComponent
	}
	c.Agent, c.ObservedAt = agent, at
	c.History = clippedHistory(c.History)
	return u.store.SavePlatform(ctx, c)
}

func clippedHistory(history []entities.PlatformDeploy) []entities.PlatformDeploy {
	kept := make([]entities.PlatformDeploy, 0, min(len(history), platformHistory))
	for _, d := range history[:min(len(history), platformHistory)] {
		d.Reason = clipRunes(d.Reason, reasonLength)
		kept = append(kept, d)
	}
	return kept
}

func clipRunes(s string, limit int) string {
	runes := []rune(s)
	return string(runes[:min(len(runes), limit)])
}

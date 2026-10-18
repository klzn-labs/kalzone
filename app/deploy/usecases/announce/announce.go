package announce

import (
	"context"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

const announcedEnvironment = "prod"

type Notifier interface {
	Notify(text string)
}

type AnnounceUsecase struct {
	notifier Notifier
	baseURL  string
}

func NewAnnounceUsecase(cfg config.Config, notifier Notifier) AnnounceUsecase {
	return AnnounceUsecase{notifier: notifier, baseURL: strings.TrimRight(cfg.BaseURL, "/")}
}

func (u AnnounceUsecase) Handle(_ context.Context, e events.Event) error {
	if e.Action != "deploy.succeeded" || e.Environment != announcedEnvironment {
		return nil
	}
	u.notifier.Notify(u.notice(e))
	return nil
}

func (u AnnounceUsecase) notice(e events.Event) string {
	return fmt.Sprintf("%s r%v is live on %s (deployment #%v, %v): %s/services/%s/env/%s",
		e.TargetName, e.Details["release"], e.Environment, e.Details["deployment"], e.Details["trigger"],
		u.baseURL, e.TargetName, e.Environment)
}

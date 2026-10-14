package logs

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/observe/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/clock"
)

type Services interface {
	FindService(ctx context.Context, slug string) (entities.LogSource, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Loki interface {
	LogLines(ctx context.Context, uid, query string, start, end time.Time, limit int) ([]entities.LogLine, error)
}

type LogsUsecase struct {
	services Services
	authz    Authorizer
	loki     Loki
	uid      string
}

func NewLogsUsecase(services Services, authz Authorizer, loki Loki, uid string) LogsUsecase {
	return LogsUsecase{services: services, authz: authz, loki: loki, uid: uid}
}

func (u LogsUsecase) Read(ctx context.Context, a actor.Actor, serviceSlug, environment string, q Query) ([]entities.LogLine, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if err := u.authz.Can(ctx, a, authz.ReadLogs, authz.Resource{TeamID: service.OwnerTeamID, Environment: environment}); err != nil {
		return nil, err
	}
	end := clock.Now()
	return u.loki.LogLines(ctx, u.uid, logQL(service.Slug, environment, q), end.Add(-q.Since), end, q.Limit)
}

func logQL(serviceSlug, environment string, q Query) string {
	colour := "(blue|green)"
	if q.Colour != "" {
		colour = q.Colour
	}
	selector := "{container=~" + strconv.Quote(regexp.QuoteMeta(serviceSlug+"-"+environment)+"-"+colour) + "}"
	if q.Contains == "" {
		return selector
	}
	return selector + " |= " + strconv.Quote(q.Contains)
}

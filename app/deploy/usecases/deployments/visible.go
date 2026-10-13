package deployments

import (
	"context"
	"strconv"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/page"
)

var (
	errNotVisible = apperror.NotFound("deployment not found").
			WithHint("list deployments with GET /api/v1/deployments?service=<slug>&env=<env>")
	errServiceNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
	errServiceRequired   = apperror.Invalid("service is required").WithField("service").
				WithHint("send ?service=<slug>; list services with GET /api/v1/services")
	errEnvironmentRequired = apperror.Invalid("env is required").WithField("env").
				WithHint("send ?env=<environment>; list environments with GET /api/v1/environments")
	errTail = apperror.Invalid("tail must be a whole number from 1 to " + strconv.Itoa(page.MaxLimit)).WithField("tail").
		WithHint("leave tail out to read from the first line")
	errTailWithCursor = apperror.Invalid("tail and cursor cannot be combined").WithField("tail").
				WithHint("use tail once, then follow with next_cursor")
)

func (u DeploymentsUsecase) PageFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, p page.Request) (page.Page[entities.Deployment], error) {
	service, err := u.visibleService(ctx, a, serviceSlug, environment)
	if err != nil {
		return page.Page[entities.Deployment]{}, err
	}
	before, err := p.Number()
	if err != nil {
		return page.Page[entities.Deployment]{}, err
	}
	list, err := u.store.History(ctx, service.ID, environment, before, p.Fetch())
	return page.Cut(list, p, deploymentKey), err
}

func (u DeploymentsUsecase) SummaryFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, entities.Summary, error) {
	d, err := u.visible(ctx, a, id)
	if err != nil {
		return entities.Deployment{}, entities.Summary{}, err
	}
	recent, err := u.store.Events(ctx, id, entities.SummaryWindow)
	if err != nil {
		return entities.Deployment{}, entities.Summary{}, err
	}
	total, err := u.store.CountEvents(ctx, id)
	return d, entities.Summarize(d, recent, total, clock.Now().UTC()), err
}

func (u DeploymentsUsecase) EventsFor(ctx context.Context, a actor.Actor, id string, p page.Request, tail string) (page.Page[entities.Event], error) {
	d, err := u.visible(ctx, a, id)
	if err != nil {
		return page.Page[entities.Event]{}, err
	}
	logged, err := u.lines(ctx, id, p, tail)
	if err != nil {
		return page.Page[entities.Event]{}, err
	}
	return follow(logged, d, p.After), nil
}

func (u DeploymentsUsecase) visible(ctx context.Context, a actor.Actor, id string) (entities.Deployment, error) {
	d, err := u.store.FindByID(ctx, id)
	if err != nil {
		return entities.Deployment{}, err
	}
	service, err := u.services.FindServiceByID(ctx, d.ServiceID)
	if err != nil {
		return entities.Deployment{}, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Deployment{}, errNotVisible
	}
	return d, nil
}

func (u DeploymentsUsecase) visibleService(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.ServiceRef, error) {
	switch {
	case serviceSlug == "":
		return entities.ServiceRef{}, errServiceRequired
	case environment == "":
		return entities.ServiceRef{}, errEnvironmentRequired
	}
	service, err := u.services.FindService(ctx, serviceSlug)
	if err == nil && !a.Sees(service.OwnerTeamID) {
		return entities.ServiceRef{}, errServiceNotVisible
	}
	return service, err
}

func (u DeploymentsUsecase) lines(ctx context.Context, id string, p page.Request, tail string) (page.Page[entities.Event], error) {
	if tail != "" {
		return u.tail(ctx, id, p, tail)
	}
	after, err := p.Number()
	if err != nil {
		return page.Page[entities.Event]{}, err
	}
	list, err := u.store.EventsAfter(ctx, id, int64(after), p.Fetch())
	return page.Cut(list, p, eventKey), err
}

func (u DeploymentsUsecase) tail(ctx context.Context, id string, p page.Request, raw string) (page.Page[entities.Event], error) {
	if p.After != "" {
		return page.Page[entities.Event]{}, errTailWithCursor
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > page.MaxLimit {
		return page.Page[entities.Event]{}, errTail
	}
	list, err := u.store.Events(ctx, id, n)
	return page.Page[entities.Event]{Items: list}, err
}

func follow(logged page.Page[entities.Event], d entities.Deployment, after string) page.Page[entities.Event] {
	if logged.Next != "" || !d.Active() {
		return logged
	}
	if len(logged.Items) == 0 {
		logged.Next = page.Encode(startOr(after))
		return logged
	}
	logged.Next = page.Encode(eventKey(logged.Items[len(logged.Items)-1]))
	return logged
}

func startOr(after string) string {
	if after == "" {
		return "0"
	}
	return after
}

func deploymentKey(d entities.Deployment) string {
	return strconv.Itoa(d.Number)
}

func eventKey(e entities.Event) string {
	return strconv.FormatInt(e.ID, 10)
}

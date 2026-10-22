package query

import (
	"context"

	"github.com/klzn-labs/kalzone/app/audit/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/page"
)

func (u QueryUsecase) ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error) {
	filter.TeamIDs, filter.AllTeams = a.TeamScope()
	filter.ViewerID = a.UserID
	return u.List(ctx, filter)
}

func (u QueryUsecase) PageFor(ctx context.Context, a actor.Actor, filter entities.Filter, p page.Request) (page.Page[entities.Event], error) {
	before, beforeID, err := p.Time()
	if err != nil {
		return page.Page[entities.Event]{}, err
	}
	filter.Before, filter.BeforeID, filter.Limit = before, beforeID, p.Fetch()
	list, err := u.ListFor(ctx, a, filter)
	return page.Cut(list, p, eventKey), err
}

func eventKey(e entities.Event) string {
	return page.TimeKey(e.At, e.ID)
}

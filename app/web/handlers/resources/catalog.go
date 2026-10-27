package resources

import (
	"context"

	resources "github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Usage struct {
	Total         int
	ByEnvironment map[string]int
}

type catalogView struct {
	Usage map[string]Usage
}

func (h ResourcesHandler) catalog(ctx context.Context, a actor.Actor) (catalogView, error) {
	list, err := h.inventory.ListFor(ctx, a, "", "")
	if err != nil {
		return catalogView{}, err
	}
	return catalogView{Usage: tally(list)}, nil
}

func tally(list []resources.Resource) map[string]Usage {
	usage := map[string]Usage{}
	for _, r := range list {
		usage[string(r.Kind)] = usage[string(r.Kind)].with(r.Environment)
	}
	return usage
}

func (u Usage) with(environment string) Usage {
	counts := map[string]int{environment: 1}
	for env, n := range u.ByEnvironment {
		counts[env] += n
	}
	return Usage{Total: u.Total + 1, ByEnvironment: counts}
}

package resources

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type Source interface {
	Resources(ctx context.Context, a actor.Actor, environment string) ([]entities.Resource, error)
}

var knownKinds = []entities.Kind{entities.KindQueue, entities.KindDatabase, entities.KindCron}

type Sources map[entities.Kind]Source

type ResourcesUsecase struct {
	sources Sources
}

func NewResourcesUsecase(sources Sources) ResourcesUsecase {
	return ResourcesUsecase{sources: sources}
}

func (u ResourcesUsecase) ListFor(ctx context.Context, a actor.Actor, kind entities.Kind, environment string) ([]entities.Resource, error) {
	chosen, err := u.chosen(kind)
	if err != nil {
		return nil, err
	}
	result := []entities.Resource{}
	for _, source := range chosen {
		list, err := source.Resources(ctx, a, environment)
		if err != nil {
			return nil, err
		}
		result = append(result, list...)
	}
	slices.SortFunc(result, compare)
	return result, nil
}

func (u ResourcesUsecase) chosen(kind entities.Kind) ([]Source, error) {
	if kind == "" {
		return slices.Collect(maps.Values(u.sources)), nil
	}
	if source, ok := u.sources[kind]; ok {
		return []Source{source}, nil
	}
	if slices.Contains(knownKinds, kind) {
		return nil, nil
	}
	return nil, apperror.Invalid("unknown resource kind " + string(kind)).WithField("kind").WithHint("send one of queue, database, cron")
}

func compare(a, b entities.Resource) int {
	return cmp.Or(cmp.Compare(a.Environment, b.Environment), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Name, b.Name))
}

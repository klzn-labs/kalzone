package overview

import (
	"sort"
	"time"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type Stage struct {
	Release int
	At      *time.Time
}

type Lane struct {
	Service catalog.Service
	Team    team.Team
	From    Stage
	To      Stage
}

func (l Lane) Behind() int {
	if l.From.Release == 0 || l.To.Release == 0 || l.To.Release >= l.From.Release {
		return 0
	}
	return l.From.Release - l.To.Release
}

func (l Lane) NotPromoted() bool {
	return l.From.Release > 0 && l.To.Release == 0
}

func (l Lane) Waits() bool {
	return l.Behind() > 0 || l.NotPromoted()
}

func promotionPair(envs []catalog.Environment) (catalog.Environment, catalog.Environment, bool) {
	ordered := append([]catalog.Environment(nil), envs...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	for i, env := range ordered {
		if env.Protected && i > 0 {
			return ordered[i-1], env, true
		}
	}
	return catalog.Environment{}, catalog.Environment{}, false
}

func sortLanes(lanes []Lane) {
	sort.SliceStable(lanes, func(i, j int) bool {
		if lanes[i].Waits() != lanes[j].Waits() {
			return lanes[i].Waits()
		}
		return lanes[i].Service.Name < lanes[j].Service.Name
	})
}

func waiting(lanes []Lane) int {
	count := 0
	for _, l := range lanes {
		if l.Waits() {
			count++
		}
	}
	return count
}

func inStep(lanes []Lane) int {
	count := 0
	for _, l := range lanes {
		if l.From.Release > 0 && l.To.Release > 0 && !l.Waits() {
			count++
		}
	}
	return count
}

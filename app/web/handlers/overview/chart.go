package overview

import (
	"sort"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
)

const chartDays = 14

type Day struct {
	Label     string
	Count     int
	Height    int
	ShowLabel bool
}

type Tick struct {
	Value  int
	Bottom int
}

type Chart struct {
	Days   []Day
	Ticks  []Tick
	Total  int
	Failed int
	Median time.Duration
}

func chartOf(ds []deploy.Deployment, today time.Time) Chart {
	start := midnight(today).AddDate(0, 0, 1-chartDays)
	counts, kept := make([]int, chartDays), []deploy.Deployment{}
	for _, d := range ds {
		day := int(midnight(*d.FinishedAt).Sub(start).Hours() / 24)
		if day >= 0 && day < chartDays {
			counts[day]++
			kept = append(kept, d)
		}
	}
	c := Chart{Total: len(kept), Failed: failed(kept), Median: medianDuration(kept)}
	c.Ticks, c.Days = scale(counts, start)
	return c
}

func failed(ds []deploy.Deployment) int {
	count := 0
	for _, d := range ds {
		if d.Status == deploy.StatusFailed {
			count++
		}
	}
	return count
}

func midnight(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func scale(counts []int, start time.Time) ([]Tick, []Day) {
	step, top := axis(counts)
	ticks := []Tick{}
	for v := 0; v <= top; v += step {
		ticks = append(ticks, Tick{Value: v, Bottom: v * 100 / top})
	}
	days := make([]Day, 0, len(counts))
	for i, n := range counts {
		label := start.AddDate(0, 0, i).Format("2 Jan")
		days = append(days, Day{Label: label, Count: n, Height: n * 100 / top, ShowLabel: (len(counts)-1-i)%3 == 0})
	}
	return ticks, days
}

func axis(counts []int) (int, int) {
	highest := 0
	for _, n := range counts {
		highest = max(highest, n)
	}
	step := 1
	for _, s := range []int{1, 2, 5, 10, 20, 50, 100} {
		step = s
		if highest/s < 4 {
			break
		}
	}
	return step, (highest/step + 1) * step
}

func medianDuration(ds []deploy.Deployment) time.Duration {
	took := []time.Duration{}
	for _, d := range ds {
		if d.Status == deploy.StatusSucceeded && d.StartedAt != nil && d.FinishedAt != nil {
			took = append(took, d.FinishedAt.Sub(*d.StartedAt))
		}
	}
	if len(took) == 0 {
		return 0
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	return took[len(took)/2].Round(time.Second)
}

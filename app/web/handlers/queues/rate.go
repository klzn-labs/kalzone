package queues

import (
	"fmt"
	"math"
	"strconv"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

const fullSpeed = 0.99

type RateView struct {
	Now        string
	Hint       string
	Adaptive   bool
	Badge      string
	BadgeClass string
	Line       string
	Reading    bool
	Over       bool
	CPUWidth   float64
}

func rateView(r Row) RateView {
	s := r.Queue.Settings
	v := RateView{Now: "—", Hint: "of " + perSecond(s.RatePerSecond) + ", fixed", Adaptive: s.Adaptive}
	if r.Fresh {
		v.Now = perSecond(r.Stats.EffectiveRate)
	}
	if !s.Adaptive {
		return v
	}
	v.Hint = "of " + perSecond(s.RatePerSecond) + ", adaptive"
	return adaptiveView(v, r)
}

func adaptiveView(v RateView, r Row) RateView {
	a, s := r.Stats.Adaptive, r.Queue.Settings
	switch {
	case !r.Fresh || a == nil:
		v.Badge, v.BadgeClass = "Adaptive", "badge-outline"
		v.Line = "The queue gateway is not slowing this queue down right now."
	case a.FailSafe:
		v.Badge, v.BadgeClass = "Adaptive · no CPU reading", "badge-danger"
		v.Line = "Database CPU unknown; climbing back to " + perSecond(s.RatePerSecond) + "."
	default:
		v.Badge, v.BadgeClass = speedBadge(a.Factor)
		v = withReading(v, *a, s)
	}
	return v
}

func speedBadge(factor float64) (string, string) {
	if factor < fullSpeed {
		return "Adaptive · slowing down", "badge-warn"
	}
	return "Adaptive · full speed", "badge-ok"
}

func withReading(v RateView, a entities.Adaptive, s entities.Settings) RateView {
	scale := math.Max(2*a.TargetCPU, 1)
	v.Reading, v.Over = true, a.DatabaseCPU > a.TargetCPU
	v.CPUWidth = math.Round(math.Min(a.DatabaseCPU/scale, 1) * 100)
	v.Line = fmt.Sprintf("DB CPU %s%% of a core, target %s%% · never below %s", decimal(a.DatabaseCPU), decimal(a.TargetCPU), perSecond(s.MinRatePerSecond))
	return v
}

func perSecond(rate float64) string {
	return decimal(rate) + " / s"
}

func decimal(x float64) string {
	return strconv.FormatFloat(math.Round(x*10)/10, 'f', -1, 64)
}

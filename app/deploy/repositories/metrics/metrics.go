package metrics

import (
	"context"
	"fmt"
	"math"
)

type Querier interface {
	Instant(ctx context.Context, uid, query string) (float64, bool, error)
}

type Metrics struct {
	querier Querier
	uid     string
}

func New(querier Querier, uid string) Metrics {
	return Metrics{querier: querier, uid: uid}
}

func (m Metrics) Usage(ctx context.Context, container string) (int, int, bool) {
	cpu, cpuOK := m.value(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{name=%q}[1m])) * 1000`, container))
	memory, memoryOK := m.value(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{name=%q}) / 1048576`, container))
	return cpu, memory, cpuOK && memoryOK
}

func (m Metrics) value(ctx context.Context, query string) (int, bool) {
	value, found, err := m.querier.Instant(ctx, m.uid, query)
	if err != nil || !found {
		return 0, false
	}
	return int(math.Round(value)), true
}

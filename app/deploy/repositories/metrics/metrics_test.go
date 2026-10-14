package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adrianozp/platform/app/deploy/repositories/metrics"
	"github.com/stretchr/testify/require"
)

type querier struct {
	queries []string
	answers []float64
	fail    bool
}

func (q *querier) Instant(_ context.Context, uid, query string) (float64, bool, error) {
	q.queries = append(q.queries, uid+" "+query)
	if q.fail {
		return 0, false, errors.New("down")
	}
	answer := q.answers[0]
	q.answers = q.answers[1:]
	return answer, true, nil
}

func Test_Usage_AsksPrometheusForCPUAndMemory(t *testing.T) {
	q := &querier{answers: []float64{2.6, 12.4}}

	cpu, memory, ok := metrics.New(q, "prom").Usage(context.Background(), "api-dev-blue")

	require.True(t, ok)
	require.Equal(t, 3, cpu)
	require.Equal(t, 12, memory)
	require.Equal(t, []string{
		`prom sum(rate(container_cpu_usage_seconds_total{name="api-dev-blue"}[1m])) * 1000`,
		`prom sum(container_memory_working_set_bytes{name="api-dev-blue"}) / 1048576`,
	}, q.queries)
}

func Test_Usage_IsUnknownWhenPrometheusFails(t *testing.T) {
	_, _, ok := metrics.New(&querier{fail: true}, "prom").Usage(context.Background(), "api-dev-blue")

	require.False(t, ok)
}

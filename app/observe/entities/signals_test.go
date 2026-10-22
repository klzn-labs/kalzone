package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_WindowNamed_FallsBackToSixHours(t *testing.T) {
	require.Equal(t, time.Hour, entities.WindowNamed("1h").Span)
	require.Equal(t, "6h", entities.WindowNamed("30d").Name)
}

func Test_SignalNamed_FallsBackToLatency(t *testing.T) {
	require.Equal(t, entities.SignalMemory, entities.SignalNamed("memory"))
	require.Equal(t, entities.SignalLatency, entities.SignalNamed("cpu"))
}

func Test_SignalQuery_ReadsTheServicesOwnContainersInOneEnvironment(t *testing.T) {
	requests := entities.SignalQuery(entities.SignalRequests, "places-api", "dev", 5*time.Minute)
	memory := entities.SignalQuery(entities.SignalMemory, "places-api", "prod", time.Minute)

	require.Equal(t, `sum(increase(traefik_service_requests_total{service=~"places-api-dev-(blue|green)@file"}[5m]))`, requests)
	require.Equal(t, `sum(container_memory_working_set_bytes{name=~"places-api-prod-(blue|green)"}) / 1048576`, memory)
	require.Contains(t, entities.SignalQuery(entities.SignalErrors, "places-api", "dev", time.Minute), `code=~"5.."`)
	require.Contains(t, entities.SignalQuery(entities.SignalLatency, "places-api", "dev", time.Minute), "* 1000")
}

func Test_SyntheticSeries_IsDeterministicAndFillsTheWindow(t *testing.T) {
	end := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	w := entities.WindowNamed("6h")

	first := entities.SyntheticSeries(entities.SignalRequests, "demo-shop-api-prod", w, end)
	again := entities.SyntheticSeries(entities.SignalRequests, "demo-shop-api-prod", w, end)
	other := entities.SyntheticSeries(entities.SignalRequests, "demo-mailer-prod", w, end)

	require.Len(t, first, 72)
	require.Equal(t, first, again)
	require.NotEqual(t, first, other)
	require.Equal(t, end, first[len(first)-1].At)
	for _, s := range first {
		require.GreaterOrEqual(t, s.Value, 0.0)
	}
}

func Test_SyntheticLines_EndAtTheGivenTimeAndRespectTheLimit(t *testing.T) {
	end := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

	lines := entities.SyntheticLines("demo-shop-api-dev", end, 10)

	require.Len(t, lines, 10)
	require.True(t, lines[9].At.Before(end))
	require.Contains(t, lines[0].Line, "GET /api/products")
}

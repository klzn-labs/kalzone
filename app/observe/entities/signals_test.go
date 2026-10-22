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

package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/stretchr/testify/require"
)

func Test_EveryFailureClassHasAHint(t *testing.T) {
	for _, class := range entities.FailureClasses {
		require.NotEmpty(t, entities.Failure{Class: class}.Hint(), class)
	}
	require.Equal(t, entities.Failure{Class: entities.FailurePlatform}.Hint(), entities.Failure{Class: "new"}.Hint())
}

func Test_ClippedKeepsTheLastLinesWithinTheirLimits(t *testing.T) {
	logs := make([]string, 60)
	for i := range logs {
		logs[i] = strings.Repeat("x", 2500)
	}
	logs[59] = "last"
	f := entities.Failure{Logs: logs, HealthLog: []string{"a", "b", "c", strings.Repeat("y", 600)}}.Clipped()

	require.Len(t, f.Logs, 50)
	require.Equal(t, "last", f.Logs[49])
	require.Len(t, f.Logs[0], 2000)
	require.Equal(t, []string{"b", "c", strings.Repeat("y", 500)}, f.HealthLog)
}

func Test_AStatusIsStaleAfterAMinute(t *testing.T) {
	now := time.Now()
	s := entities.RuntimeStatus{ObservedAt: now.Add(-61 * time.Second)}

	require.True(t, s.Stale(now))
	require.False(t, entities.RuntimeStatus{ObservedAt: now.Add(-30 * time.Second)}.Stale(now))
}

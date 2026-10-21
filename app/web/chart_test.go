package web_test

import (
	"testing"
	"time"

	observe "github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/stretchr/testify/require"
)

func Test_MetricChartDrawsTheSeriesUnderItsLimit(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	r := observe.ReadingOf(observe.MetricDBCPU, 70, []observe.Sample{{At: at, Value: 0}, {At: at.Add(5 * time.Minute), Value: 80.5}}, 5*time.Minute)

	svg := string(web.MetricChart(r))

	require.Contains(t, svg, `aria-label="DB CPU over the last 24 hours"`)
	require.Contains(t, svg, `d="M0.0 74.0 L640.0 14.9"`)
	require.Contains(t, svg, `class="limit"`)
	require.Empty(t, web.MetricChart(observe.Reading{}))
	require.Equal(t, 81, web.Rounded(80.5))
}

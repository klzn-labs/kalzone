package observability

import (
	"testing"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

var end = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func samples(values ...float64) []entities.Sample {
	result := make([]entities.Sample, 0, len(values))
	for i, v := range values {
		result = append(result, entities.Sample{At: end.Add(-time.Duration(len(values)-1-i) * time.Hour), Value: v})
	}
	return result
}

func sixHours(series map[entities.Signal][]entities.Sample) inputs {
	w := entities.WindowNamed("6h")
	s := state{base: "/services/api/env/dev/observability", window: w, signal: entities.SignalLatency, logs: "errors"}
	return inputs{state: s, metrics: entities.Metrics{Window: w, End: end, Series: series}, limits: limits{latencyMS: 300, memoryMB: 512}}
}

func Test_Tiles_SumRequestsAndShowTheLatestValueOfTheRest(t *testing.T) {
	in := sixHours(map[entities.Signal][]entities.Sample{
		entities.SignalRequests: samples(1200, 800.4, 2371), entities.SignalErrors: samples(0.002, 0.049, 0.002),
		entities.SignalLatency: samples(120, 489, 168),
	})

	tiles := tilesOf(in)

	require.Equal(t, "4,371", tiles[0].Value)
	require.Equal(t, "in 6h", tiles[0].Note)
	require.Equal(t, "0.2%", tiles[1].Value)
	require.Equal(t, "peak 4.9%", tiles[1].Note)
	require.True(t, tiles[1].Hot)
	require.Equal(t, "168 ms", tiles[2].Value)
	require.Equal(t, "limit 300 ms", tiles[2].Note)
	require.True(t, tiles[2].Hot)
	require.True(t, tiles[2].Selected)
	require.Equal(t, "—", tiles[3].Value)
	require.Equal(t, "no data", tiles[3].Note)
	require.Equal(t, "/services/api/env/dev/observability?logs=errors&signal=memory&window=6h", tiles[3].Href)
}

func Test_Chart_ScalesToThePeakOrTheLimitAndMarksDeploys(t *testing.T) {
	in := sixHours(map[entities.Signal][]entities.Sample{entities.SignalLatency: samples(100, 200)})
	finished, before := end.Add(-3*time.Hour), end.Add(-7*time.Hour)
	in.history = []deploy.Deployment{{ReleaseNumber: 42, FinishedAt: &finished}, {ReleaseNumber: 41, FinishedAt: &before}, {ReleaseNumber: 43}}

	c := chartOf(in)

	require.Equal(t, "M533.3,127.8 L640.0,75.7", c.Path)
	require.True(t, c.HasLimit)
	require.InDelta(t, 23.5, c.LimitY, 0.1)
	require.Equal(t, []Marker{{X: 320, Label: "r42 · 09:00"}}, c.Markers)
	require.Equal(t, "peak 200 ms at 12:00 · limit 300 ms", c.Note)
	require.Equal(t, []string{"345 ms", "172 ms", "0 ms"}, c.Ticks)
	require.Equal(t, []string{"06:00", "08:00", "10:00", "12:00"}, c.Axis)
}

func Test_Chart_SaysWhenAWindowHasNoData(t *testing.T) {
	c := chartOf(sixHours(map[entities.Signal][]entities.Sample{}))

	require.True(t, c.Empty)
	require.Empty(t, c.Path)
	require.Equal(t, "No data in this window", c.Note)
}

func Test_LogRows_ListNewestFirstAndToneErrorsAndWarnings(t *testing.T) {
	lines := []entities.LogLine{
		{At: end.Add(-2 * time.Second), Line: "warn: pool exhausted"},
		{At: end.Add(-time.Second), Line: "GET /places 503 2.9s"},
		{At: end, Line: "GET /places 200 41ms"},
	}

	rows := logRows(lines)

	require.Equal(t, []LogRow{
		{At: "12:00:00", Line: "GET /places 200 41ms"},
		{At: "11:59:59", Line: "GET /places 503 2.9s", Tone: "error"},
		{At: "11:59:58", Line: "warn: pool exhausted", Tone: "warn"},
	}, rows)
}

func Test_WindowLinksKeepTheSignalAndLogMode(t *testing.T) {
	links := windowLinks(sixHours(nil).state)

	require.Len(t, links, 4)
	require.Equal(t, "/services/api/env/dev/observability?logs=errors&signal=latency&window=1h", links[0].Href)
	require.True(t, links[1].Active)
}

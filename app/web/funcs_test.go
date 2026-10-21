package web_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

func Test_Ago(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	clock.WithTime(now)

	require.Equal(t, "just now", web.Ago(now.Add(-10*time.Second)))
	require.Equal(t, "1 minute ago", web.Ago(now.Add(-time.Minute)))
	require.Equal(t, "3 hours ago", web.Ago(now.Add(-3*time.Hour)))
	require.Equal(t, "2 days ago", web.Ago(now.Add(-49*time.Hour)))
	require.Equal(t, "Jan 1, 2026", web.Ago(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func Test_Left(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	clock.WithTime(now)

	require.Equal(t, "23 hours", web.Left(now.Add(23*time.Hour+31*time.Minute)))
	require.Equal(t, "1 minute", web.Left(now.Add(90*time.Second)))
	require.Equal(t, "under a minute", web.Left(now.Add(10*time.Second)))
	require.Equal(t, "no time", web.Left(now.Add(-time.Second)))
}

func Test_Initials(t *testing.T) {
	require.Equal(t, "PA", web.Initials("Places API"))
	require.Equal(t, "PA", web.Initials("places-api"))
	require.Equal(t, "?", web.Initials(""))
}

func Test_Span_DropsZeroTails(t *testing.T) {
	require.Equal(t, "168h", web.Span(168*time.Hour))
	require.Equal(t, "1m", web.Span(time.Minute))
	require.Equal(t, "1m30s", web.Span(90*time.Second))
	require.Equal(t, "2h5m", web.Span(2*time.Hour+5*time.Minute))
	require.Equal(t, "500ms", web.Span(500*time.Millisecond))
}

func Test_RendererParsesEveryPage(t *testing.T) {
	_, err := web.NewRenderer()

	require.NoError(t, err)
}

func Test_Took(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 51, 52, 0, time.UTC)
	end := start.Add(2*time.Minute + 2*time.Second + 300*time.Millisecond)

	require.Equal(t, "2m2s", web.Took(&start, &end))
	require.Equal(t, "", web.Took(nil, &end))
}

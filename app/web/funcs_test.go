package web_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/pkg/clock"
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

func Test_Initials(t *testing.T) {
	require.Equal(t, "PA", web.Initials("Places API"))
	require.Equal(t, "PA", web.Initials("places-api"))
	require.Equal(t, "?", web.Initials(""))
}

func Test_RendererParsesEveryPage(t *testing.T) {
	_, err := web.NewRenderer()

	require.NoError(t, err)
}

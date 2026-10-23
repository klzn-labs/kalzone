package cronspec_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/cronspec"
	"github.com/stretchr/testify/require"
)

func Test_Parse_AcceptsFiveFieldsAndShortcuts(t *testing.T) {
	for _, spec := range []string{"* * * * *", "*/5 * * * *", "0 3 * * 1-5", "30 2 1 * *", "@hourly", "@daily", " 0 * * * * "} {
		_, err := cronspec.Parse(spec)
		require.NoError(t, err, spec)
	}
}

func Test_Parse_RefusesOtherForms(t *testing.T) {
	for _, spec := range []string{"", "* * * *", "0 * * * * *", "@every 1m", "CRON_TZ=Europe/Lisbon 0 * * * *", "TZ=UTC 0 * * * *", "61 * * * *", "@reboot", "nonsense"} {
		_, err := cronspec.Parse(spec)
		require.ErrorIs(t, err, apperror.Invalid(""), spec)
	}
}

func Test_Next_IsInUTC(t *testing.T) {
	lisbon := time.FixedZone("UTC+1", 3600)
	s, err := cronspec.Parse("0 3 * * *")
	require.NoError(t, err)

	next := s.Next(time.Date(2026, 10, 23, 3, 30, 0, 0, lisbon))

	require.Equal(t, time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC), next)
	require.Equal(t, time.UTC, next.Location())
}

func Test_Next_DailyShortcutIsMidnightUTC(t *testing.T) {
	s, err := cronspec.Parse("@daily")
	require.NoError(t, err)

	require.Equal(t, time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), s.Next(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)))
	require.Equal(t, "@daily", s.String())
}

func Test_Next_EveryMinuteIsTheNextWholeMinute(t *testing.T) {
	s, err := cronspec.Parse("* * * * *")
	require.NoError(t, err)

	require.Equal(t, time.Date(2026, 10, 23, 12, 1, 0, 0, time.UTC), s.Next(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)))
}

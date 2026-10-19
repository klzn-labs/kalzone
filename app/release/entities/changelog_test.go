package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/stretchr/testify/require"
)

func Test_ParseCommit_ReadsConventionalType(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cases := map[string][2]string{
		"feat(lists): share\n\nbody": {"feat(lists): share", "feat"},
		"Fix: x":                     {"Fix: x", "fix"},
		"tidy logs":                  {"tidy logs", "other"},
		"feat!: drop v1":             {"feat!: drop v1", "feat"},
		"wip: later":                 {"wip: later", "other"},
	}
	for message, want := range cases {
		c := entities.ParseCommit("abc", message, "ana", at)
		require.Equal(t, want[0], c.Title, message)
		require.Equal(t, want[1], c.Type, message)
		require.Equal(t, entities.Commit{SHA: "abc", Title: want[0], Type: want[1], Author: "ana", CommittedAt: at}, c)
	}
}

func Test_RetryDue_BacksOffExponentially(t *testing.T) {
	now := time.Now()
	c := entities.Changelog{State: entities.ChangelogPending, Attempts: 2}

	c.UpdatedAt = now.Add(-3 * time.Minute)
	require.False(t, c.RetryDue(now))
	c.UpdatedAt = now.Add(-5 * time.Minute)
	require.True(t, c.RetryDue(now))
}

func Test_RetryDue_NeverWhileAFirstFetchMayStillRun(t *testing.T) {
	now := time.Now()
	c := entities.Changelog{State: entities.ChangelogPending}

	c.UpdatedAt = now.Add(-2 * time.Minute)
	require.False(t, c.RetryDue(now))
	c.UpdatedAt = now.Add(-3 * time.Minute)
	require.True(t, c.RetryDue(now))
}

func Test_RetryDue_OnlyForPending(t *testing.T) {
	c := entities.Changelog{State: entities.ChangelogAhead, UpdatedAt: time.Now().Add(-time.Hour)}

	require.False(t, c.RetryDue(time.Now()))
}

package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/stretchr/testify/require"
)

func Test_ValidateName(t *testing.T) {
	for _, ok := range []string{"place-import", "ab", "q1"} {
		require.NoError(t, entities.ValidateName(ok), ok)
	}
	for _, bad := range []string{"", "a", "Place", "1queue", "has_underscore", string(make([]byte, 64))} {
		require.Error(t, entities.ValidateName(bad), bad)
	}
}

func Test_Settings_DefaultsAreValid(t *testing.T) {
	require.NoError(t, entities.DefaultSettings("/hooks/x").Validate())
}

func Test_Settings_Limits(t *testing.T) {
	broken := map[string]func(s *entities.Settings){
		"path":              func(s *entities.Settings) { s.Path = "hooks" },
		"long path":         func(s *entities.Settings) { s.Path = "/" + string(make([]byte, 255)) },
		"rate low":          func(s *entities.Settings) { s.RatePerSecond = 0.05 },
		"rate high":         func(s *entities.Settings) { s.RatePerSecond = 1001 },
		"in flight":         func(s *entities.Settings) { s.MaxInFlight = 0 },
		"in flight high":    func(s *entities.Settings) { s.MaxInFlight = 101 },
		"attempts":          func(s *entities.Settings) { s.MaxAttempts = 51 },
		"backoff kind":      func(s *entities.Settings) { s.Backoff.Kind = "linear" },
		"backoff initial":   func(s *entities.Settings) { s.Backoff.Initial = 50 * time.Millisecond },
		"backoff max<init":  func(s *entities.Settings) { s.Backoff.Max = 500 * time.Millisecond },
		"backoff max high":  func(s *entities.Settings) { s.Backoff.Max = 7 * time.Hour },
		"timeout":           func(s *entities.Settings) { s.DeliveryTimeout = 16 * time.Minute },
		"retention low":     func(s *entities.Settings) { s.Retention = 30 * time.Minute },
		"retention high":    func(s *entities.Settings) { s.Retention = 721 * time.Hour },
		"message too small": func(s *entities.Settings) { s.MaxMessageBytes = 512 },
		"message too large": func(s *entities.Settings) { s.MaxMessageBytes = 2 << 20 },
	}
	for name, breakIt := range broken {
		s := entities.DefaultSettings("/hooks/x")
		breakIt(&s)
		require.Error(t, s.Validate(), name)
	}
}

func Test_Settings_AdaptiveMinimum(t *testing.T) {
	adaptive := func(min float64) entities.Settings {
		s := entities.DefaultSettings("/hooks/x")
		s.Adaptive, s.MinRatePerSecond = true, min
		return s
	}
	require.NoError(t, adaptive(0.1).Validate())
	require.NoError(t, adaptive(10).Validate(), "a minimum equal to the rate is a constant rate")
	require.Error(t, adaptive(0.05).Validate())
	require.Error(t, adaptive(10.5).Validate())
}

func Test_Settings_FixedQueueIgnoresMinimum(t *testing.T) {
	s := entities.DefaultSettings("/hooks/x")
	s.MinRatePerSecond = 0
	require.NoError(t, s.Validate(), "versions stored before adaptive rates have no minimum")
}

func Test_Backoff_Delay(t *testing.T) {
	expo := entities.Backoff{Kind: entities.BackoffExponential, Initial: time.Second, Max: 10 * time.Second}
	fixed := entities.Backoff{Kind: entities.BackoffFixed, Initial: 3 * time.Second, Max: 3 * time.Second}

	require.Equal(t, time.Second, expo.Delay(1))
	require.Equal(t, 4*time.Second, expo.Delay(3))
	require.Equal(t, 10*time.Second, expo.Delay(10))
	require.Equal(t, 10*time.Second, expo.Delay(200))
	require.Equal(t, 3*time.Second, fixed.Delay(7))
}

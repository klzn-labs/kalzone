package model_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/stretchr/testify/require"
)

func Test_EffectiveRate_MovesBetweenMinimumAndRate(t *testing.T) {
	s := model.Settings{RatePerSecond: 10, Adaptive: true, MinRatePerSecond: 2}

	require.Equal(t, 2.0, s.EffectiveRate(0))
	require.Equal(t, 6.0, s.EffectiveRate(0.5))
	require.Equal(t, 10.0, s.EffectiveRate(1))
	require.Equal(t, 10.0, s.EffectiveRate(1.7))
	require.Equal(t, 2.0, s.EffectiveRate(-3))
}

func Test_EffectiveRate_FixedQueuesIgnoreTheFactor(t *testing.T) {
	s := model.Settings{RatePerSecond: 10, MinRatePerSecond: 2}

	require.Equal(t, 10.0, s.EffectiveRate(0))
}

func Test_EffectiveRate_EqualBoundsGiveAConstantRate(t *testing.T) {
	s := model.Settings{RatePerSecond: 4, Adaptive: true, MinRatePerSecond: 4}

	require.Equal(t, 4.0, s.EffectiveRate(0))
	require.Equal(t, 4.0, s.EffectiveRate(0.3))
}

func Test_FixedThrottle_KeepsTheFullRate(t *testing.T) {
	require.Equal(t, model.AdaptiveState{Factor: 1}, model.FixedThrottle{}.Adaptive())
}

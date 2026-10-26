package model

type AdaptiveState struct {
	On       bool
	Factor   float64
	Measured float64
	Setpoint float64
	FailSafe bool
}

type Throttle interface {
	Adaptive() AdaptiveState
}

type FixedThrottle struct{}

func (FixedThrottle) Adaptive() AdaptiveState {
	return AdaptiveState{Factor: 1}
}

func (s Settings) EffectiveRate(factor float64) float64 {
	if !s.Adaptive {
		return s.RatePerSecond
	}
	factor = max(0, min(1, factor))
	return s.MinRatePerSecond + factor*(s.RatePerSecond-s.MinRatePerSecond)
}

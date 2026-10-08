package entities

import (
	"regexp"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/apperror"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

type check struct {
	ok      bool
	message string
}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid("queue name must be 2-63 lowercase letters, digits or dashes, starting with a letter")
	}
	return nil
}

func (s Settings) Validate() error {
	for _, c := range append(s.deliveryChecks(), s.Backoff.checks()...) {
		if !c.ok {
			return apperror.Invalid(c.message)
		}
	}
	return nil
}

func (s Settings) deliveryChecks() []check {
	return []check{
		{strings.HasPrefix(s.Path, "/") && len(s.Path) <= 255, "path must start with / and be at most 255 characters"},
		{s.RatePerSecond >= 0.1 && s.RatePerSecond <= 1000, "rate must be between 0.1 and 1000 messages per second"},
		{s.MaxInFlight >= 1 && s.MaxInFlight <= 100, "in flight must be between 1 and 100"},
		{s.MaxAttempts >= 1 && s.MaxAttempts <= 50, "attempts must be between 1 and 50"},
		{s.DeliveryTimeout >= time.Second && s.DeliveryTimeout <= 15*time.Minute, "delivery timeout must be between 1s and 15m"},
		{s.Retention >= time.Hour && s.Retention <= 720*time.Hour, "retention must be between 1 hour and 30 days"},
		{s.MaxMessageBytes >= 1<<10 && s.MaxMessageBytes <= 1<<20, "message size must be between 1 KiB and 1 MiB"},
	}
}

func (b Backoff) checks() []check {
	return []check{
		{b.Kind == BackoffFixed || b.Kind == BackoffExponential, "backoff must be fixed or exponential"},
		{b.Initial >= 100*time.Millisecond && b.Initial <= time.Hour, "backoff initial delay must be between 100ms and 1h"},
		{b.Max >= b.Initial && b.Max <= 6*time.Hour, "backoff max delay must be at least the initial delay and at most 6h"},
	}
}

func (b Backoff) Delay(attempt int) time.Duration {
	if b.Kind == BackoffFixed || attempt <= 1 {
		return min(b.Initial, b.Max)
	}
	delay := b.Initial
	for i := 1; i < attempt && delay < b.Max; i++ {
		delay *= 2
	}
	return min(delay, b.Max)
}

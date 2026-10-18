package dispatch_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/dispatch"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/stretchr/testify/require"
)

func Test_Outcome_Table(t *testing.T) {
	cases := map[int]dispatch.Verdict{
		200: dispatch.Ack, 204: dispatch.Ack, 400: dispatch.Bury, 404: dispatch.Bury, 422: dispatch.Bury,
		408: dispatch.Retry, 429: dispatch.Retry, 500: dispatch.Retry, 503: dispatch.Retry,
	}
	for status, want := range cases {
		got, _ := dispatch.Outcome(status, nil, http.Header{})
		require.Equal(t, want, got, status)
	}
	failed, _ := dispatch.Outcome(0, errors.New("connection refused"), nil)
	require.Equal(t, dispatch.Retry, failed)
	_, after := dispatch.Outcome(429, nil, http.Header{"Retry-After": {"7"}})
	require.Equal(t, 7*time.Second, after)
}

func Test_Delay_ExponentialFixedJitterAndRetryAfterCap(t *testing.T) {
	expo := model.Backoff{Kind: model.BackoffExponential, Initial: time.Second, Max: 10 * time.Second}
	fixed := model.Backoff{Kind: model.BackoffFixed, Initial: 3 * time.Second, Max: 3 * time.Second}
	middle := func() float64 { return 0.5 }
	low := func() float64 { return 0 }

	require.Equal(t, 4*time.Second, dispatch.Delay(expo, 3, 0, middle))
	require.Equal(t, 3200*time.Millisecond, dispatch.Delay(expo, 3, 0, low))
	require.Equal(t, 10*time.Second, dispatch.Delay(expo, 30, 0, middle))
	require.Equal(t, 3*time.Second, dispatch.Delay(fixed, 9, 0, low))
	require.Equal(t, 10*time.Second, dispatch.Delay(expo, 1, time.Hour, middle))
	require.Equal(t, 2*time.Second, dispatch.Delay(expo, 1, 2*time.Second, middle))
}

package dispatch

import (
	"net/http"
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/model"
)

type Verdict int

const (
	Ack Verdict = iota
	Retry
	Bury
)

func Outcome(status int, err error, header http.Header) (Verdict, time.Duration) {
	if err != nil {
		return Retry, 0
	}
	switch {
	case status >= 200 && status < 300:
		return Ack, 0
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500:
		return Retry, retryAfter(header)
	case status >= 400:
		return Bury, 0
	}
	return Retry, 0
}

func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(header.Get("Retry-After"))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func Delay(b model.Backoff, attempt int, after time.Duration, jitter func() float64) time.Duration {
	if after > 0 {
		return min(after, b.Max)
	}
	if b.Kind == model.BackoffFixed {
		return b.Initial
	}
	base := b.Initial
	for i := 1; i < attempt && base < b.Max; i++ {
		base *= 2
	}
	base = min(base, b.Max)
	return min(time.Duration(float64(base)*(0.8+0.4*jitter())), b.Max)
}

package wait

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

const (
	Max        = 120 * time.Second
	RetryAfter = "5"
)

var errDuration = apperror.Invalid("wait must be a duration above 0 and at most 120s, such as 30s or 2m").WithField("wait").
	WithHint("leave wait out to answer at once")

type Requests interface {
	RequestState(ctx context.Context, a actor.Actor, id string) (string, bool, error)
}

type Load[T any] func(ctx context.Context) (T, error)

type Drain struct {
	once *sync.Once
	done chan struct{}
}

func NewDrain() *Drain {
	return &Drain{once: &sync.Once{}, done: make(chan struct{})}
}

func (d *Drain) Close() {
	d.once.Do(func() { close(d.done) })
}

type Waiter struct {
	drain    *Drain
	requests Requests
	every    time.Duration
}

func NewWaiter(drain *Drain, requests Requests) Waiter {
	return Waiter{drain: drain, requests: requests, every: time.Second}
}

func Duration(c *gin.Context) (time.Duration, bool) {
	raw := c.Query("wait")
	if raw == "" {
		return 0, true
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 || d > Max {
		httperror.Abort(c, errDuration)
		return 0, false
	}
	return d, true
}

func Read[T any](c *gin.Context, w Waiter, d time.Duration, load Load[T], final func(T) bool) {
	value, err := load(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	if d == 0 {
		c.JSON(http.StatusOK, value)
		return
	}
	settle(c, w, d, value, load, final)
}

func Started[T any](c *gin.Context, w Waiter, d time.Duration, value T, err error, load Load[T], final func(T) bool) {
	var pending *approval.Pending
	switch {
	case errors.As(err, &pending) && d > 0:
		settle(c, w, d, requestState{Pending: *pending}, w.request(c, *pending), requestState.settled)
	case err != nil:
		httperror.Abort(c, err)
	case d == 0:
		c.JSON(http.StatusAccepted, value)
	default:
		settle(c, w, d, value, load, final)
	}
}

func settle[T any](c *gin.Context, w Waiter, d time.Duration, value T, load Load[T], final func(T) bool) {
	value, err := Until(c.Request.Context(), w, d, value, load, final)
	if err != nil || callerGone(c) {
		fail(c, err)
		return
	}
	respond(c, d, value, final)
}

func PollThenRead[T any](c *gin.Context, w Waiter, d time.Duration, poll, read Load[T], final func(T) bool) {
	if d > 0 && !waited(c, w, d, poll, final) {
		return
	}
	value, err := read(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, d, value, final)
}

func waited[T any](c *gin.Context, w Waiter, d time.Duration, poll Load[T], final func(T) bool) bool {
	value, err := poll(c.Request.Context())
	if err == nil {
		_, err = Until(c.Request.Context(), w, d, value, poll, final)
	}
	if err != nil || callerGone(c) {
		fail(c, err)
		return false
	}
	return true
}

func respond[T any](c *gin.Context, d time.Duration, value T, final func(T) bool) {
	if d == 0 || final(value) {
		c.JSON(http.StatusOK, value)
		return
	}
	c.Header("Retry-After", RetryAfter)
	c.JSON(http.StatusAccepted, value)
}

func fail(c *gin.Context, err error) {
	if callerGone(c) {
		c.Abort()
		return
	}
	httperror.Abort(c, err)
}

func callerGone(c *gin.Context) bool {
	return c.Request.Context().Err() != nil
}

func Until[T any](ctx context.Context, w Waiter, d time.Duration, value T, load Load[T], final func(T) bool) (T, error) {
	if final(value) {
		return value, nil
	}
	f := &follow[T]{value: value, load: load, final: final}
	err := w.poll(ctx, time.Now().Add(d), f.settled)
	return f.value, err
}

type follow[T any] struct {
	value T
	load  Load[T]
	final func(T) bool
}

func (f *follow[T]) settled(ctx context.Context) (bool, error) {
	next, err := f.load(ctx)
	if err != nil {
		return false, err
	}
	f.value = next
	return f.final(next), nil
}

func (w Waiter) poll(ctx context.Context, deadline time.Time, settled func(context.Context) (bool, error)) error {
	for w.pause(ctx, deadline) {
		if done, err := settled(ctx); done || err != nil {
			return err
		}
	}
	return nil
}

func (w Waiter) pause(ctx context.Context, deadline time.Time) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return false
	}
	timer := time.NewTimer(min(w.every, remaining))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-w.drain.done:
		return false
	case <-timer.C:
		return true
	}
}

type requestState struct {
	approval.Pending
	final bool
}

func (s requestState) settled() bool {
	return s.final
}

func (w Waiter) request(c *gin.Context, pending approval.Pending) Load[requestState] {
	a := requestctx.ActorFrom(c)
	return func(ctx context.Context) (requestState, error) {
		status, final, err := w.requests.RequestState(ctx, a, pending.ID)
		pending.Status = status
		return requestState{Pending: pending, final: final}, err
	}
}

package dispatch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/backend/jetstream"
	"github.com/klzn-labs/kalzone/internal/queued/dispatch"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/internal/queued/natstest"
	"github.com/klzn-labs/kalzone/internal/queued/signing"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type consumer struct {
	hits    atomic.Int32
	active  atomic.Int32
	peak    atomic.Int32
	mu      sync.Mutex
	headers []http.Header
	bodies  []string
}

func (c *consumer) serve(t *testing.T, respond func(n int32) (int, time.Duration)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := c.hits.Add(1)
		c.track(c.active.Add(1))
		defer c.active.Add(-1)
		body, _ := io.ReadAll(r.Body)
		c.record(r.Header, string(body))
		status, wait := respond(n)
		time.Sleep(wait)
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server
}

func (c *consumer) track(active int32) {
	for {
		peak := c.peak.Load()
		if active <= peak || c.peak.CompareAndSwap(peak, active) {
			return
		}
	}
}

func (c *consumer) record(h http.Header, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headers, c.bodies = append(c.headers, h.Clone()), append(c.bodies, body)
}

type recorder struct{ verdicts sync.Map }

func (r *recorder) Delivered(_ string, v dispatch.Verdict, _ time.Duration) {
	count, _ := r.verdicts.LoadOrStore(v, new(atomic.Int32))
	count.(*atomic.Int32).Add(1)
}

func start(t *testing.T, q model.Queue) (*jetstream.Driver, *dispatch.Dispatcher) {
	t.Helper()
	d, err := jetstream.New(natstest.Start(t))
	require.NoError(t, err)
	require.NoError(t, d.Ensure(ctx, q))
	disp := dispatch.New(d, &http.Client{}, q, &recorder{}, model.FixedThrottle{})
	disp.Start(ctx)
	t.Cleanup(disp.Stop)
	return d, disp
}

func queueFor(server *httptest.Server) model.Queue {
	q := natstest.Queue("jobs")
	q.Target = server.URL + "/hooks"
	return q
}

func depth(t *testing.T, d *jetstream.Driver, q model.Queue) int64 {
	stats, err := d.Stats(ctx, q)
	require.NoError(t, err)
	return stats.Depth
}

func Test_Dispatcher_DeliversSignedMessages(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	d, _ := start(t, q)

	id, _ := d.Publish(ctx, q, model.Outgoing{Body: []byte(`{"city":7}`), ContentType: "application/json"})

	require.Eventually(t, func() bool { return depth(t, d, q) == 0 && c.hits.Load() == 1 }, 5*time.Second, 50*time.Millisecond)
	h := c.headers[0]
	require.Equal(t, id, h.Get("Platform-Message-Id"))
	require.Equal(t, "1", h.Get("Platform-Attempt"))
	require.Equal(t, "jobs", h.Get("Platform-Queue"))
	require.Equal(t, "application/json", h.Get("Content-Type"))
	require.NotEmpty(t, h.Get("Platform-Published-At"))
	require.NoError(t, signing.Verify(h.Get("Platform-Signature"), []string{"key"}, []byte(c.bodies[0]), time.Now(), time.Minute))
}

func Test_Dispatcher_RetriesThenBuriesAfterMaxAttempts(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 500, 0 }))
	d, _ := start(t, q)

	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})

	require.Eventually(t, func() bool {
		stats, _ := d.Stats(ctx, q)
		return stats.DeadLetters == 1
	}, 5*time.Second, 50*time.Millisecond)
	dead, _ := d.Dead(ctx, q, 1)
	require.EqualValues(t, 3, c.hits.Load())
	require.Equal(t, 3, dead[0].Attempts)
	require.Equal(t, 500, dead[0].LastStatus)
	require.Equal(t, []string{"1", "2", "3"}, []string{c.headers[0].Get("Platform-Attempt"), c.headers[1].Get("Platform-Attempt"), c.headers[2].Get("Platform-Attempt")})
}

func Test_Dispatcher_PermanentFailureGoesStraightToDead(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 422, 0 }))
	d, _ := start(t, q)

	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})

	require.Eventually(t, func() bool {
		stats, _ := d.Stats(ctx, q)
		return stats.DeadLetters == 1
	}, 5*time.Second, 50*time.Millisecond)
	require.EqualValues(t, 1, c.hits.Load())
}

func Test_Dispatcher_CutsAHangingConsumerAndRetries(t *testing.T) {
	c := &consumer{}
	server := c.serve(t, func(n int32) (int, time.Duration) {
		if n == 1 {
			return 200, 2 * time.Second
		}
		return 200, 0
	})
	q := queueFor(server)
	q.Settings.DeliveryTimeout = 300 * time.Millisecond
	d, _ := start(t, q)

	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})

	require.Eventually(t, func() bool { return depth(t, d, q) == 0 }, 5*time.Second, 50*time.Millisecond)
	require.EqualValues(t, 2, c.hits.Load())
}

func Test_Dispatcher_HonoursInFlight(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 150 * time.Millisecond }))
	q.Settings.MaxInFlight = 2
	d, _ := start(t, q)

	for range 8 {
		d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	}

	require.Eventually(t, func() bool { return depth(t, d, q) == 0 }, 10*time.Second, 50*time.Millisecond)
	require.EqualValues(t, 2, c.peak.Load())
}

func Test_Dispatcher_HonoursTheRate(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	q.Settings.RatePerSecond = 4
	d, _ := start(t, q)

	for range 12 {
		d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	}
	time.Sleep(time.Second)

	require.LessOrEqual(t, c.hits.Load(), int32(6))
	require.GreaterOrEqual(t, c.hits.Load(), int32(3))
}

func Test_Dispatcher_PausedDeliversNothingUntilResumed(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	q.Paused = true
	d, disp := start(t, q)

	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	time.Sleep(700 * time.Millisecond)
	before := c.hits.Load()
	q.Paused = false
	disp.Update(q)

	require.EqualValues(t, 0, before)
	require.Eventually(t, func() bool { return c.hits.Load() == 1 }, 5*time.Second, 50*time.Millisecond)
}

func Test_Dispatcher_NeverHoldsAFetchedMessageWhileWaitingForTheRate(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	q.Settings.RatePerSecond = 4
	q.Settings.MaxInFlight = 20
	m := newMemory(8)
	disp := dispatch.New(m, &http.Client{}, q, &recorder{}, model.FixedThrottle{})
	disp.Start(ctx)
	defer disp.Stop()

	require.Eventually(t, func() bool { _, acked := m.longestHeld(); return acked == 8 }, 10*time.Second, 50*time.Millisecond)
	longest, _ := m.longestHeld()
	require.Less(t, longest, 500*time.Millisecond)
}

func Test_Dispatcher_UpdateWhileDeliveringNeitherDropsNorDuplicates(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 150 * time.Millisecond }))
	q.Settings.MaxInFlight = 1
	d, disp := start(t, q)
	for range 10 {
		d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	}
	time.Sleep(400 * time.Millisecond)

	q.Settings.MaxInFlight = 4
	disp.Update(q)

	require.Eventually(t, func() bool { return depth(t, d, q) == 0 }, 10*time.Second, 50*time.Millisecond)
	require.EqualValues(t, 10, c.hits.Load())
	require.Greater(t, c.peak.Load(), int32(1))
	require.Equal(t, 10, distinctIDs(c))
}

func distinctIDs(c *consumer) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	for _, h := range c.headers {
		seen[h.Get("Platform-Message-Id")] = true
	}
	return len(seen)
}

type stuckThrottle struct{ factor float64 }

func (s stuckThrottle) Adaptive() model.AdaptiveState {
	return model.AdaptiveState{On: true, Factor: s.factor}
}

func ackedWithin(t *testing.T, q model.Queue, throttle model.Throttle, window time.Duration) int {
	t.Helper()
	m := newMemory(100)
	disp := dispatch.New(m, &http.Client{}, q, &recorder{}, throttle)
	disp.Start(ctx)
	time.Sleep(window)
	disp.Stop()
	_, acked := m.longestHeld()
	return acked
}

func Test_Dispatcher_AnAdaptiveQueueFollowsTheFactor(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	q.Settings.RatePerSecond, q.Settings.MaxInFlight = 20, 20
	q.Settings.Adaptive, q.Settings.MinRatePerSecond = true, 2

	throttled := ackedWithin(t, q, stuckThrottle{factor: 0}, 2*time.Second)
	open := ackedWithin(t, q, stuckThrottle{factor: 1}, 2*time.Second)

	require.LessOrEqual(t, throttled, 6)
	require.GreaterOrEqual(t, open, 30)
}

func Test_Dispatcher_AFixedQueueIgnoresTheFactor(t *testing.T) {
	c := &consumer{}
	q := queueFor(c.serve(t, func(int32) (int, time.Duration) { return 200, 0 }))
	q.Settings.RatePerSecond, q.Settings.MaxInFlight = 20, 20

	require.GreaterOrEqual(t, ackedWithin(t, q, stuckThrottle{factor: 0}, 2*time.Second), 30)
}

package wait

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type thing struct {
	Status string `json:"status"`
}

func isDone(t thing) bool {
	return t.Status == "done"
}

type steps struct {
	statuses []string
	reads    int
}

func (s *steps) load(context.Context) (thing, error) {
	status := s.statuses[min(s.reads, len(s.statuses)-1)]
	s.reads++
	return thing{Status: status}, nil
}

type requests struct{ status string }

func (r requests) RequestState(context.Context, actor.Actor, string) (string, bool, error) {
	return r.status, r.status == "executed", nil
}

func fast(drain *Drain, r Requests) Waiter {
	return Waiter{drain: drain, requests: r, every: 5 * time.Millisecond}
}

func call(t *testing.T, query string, handle func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/x"+query, nil)
	handle(c)
	return recorder
}

func statusOf(t *testing.T, r *httptest.ResponseRecorder) string {
	t.Helper()
	var body thing
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &body))
	return body.Status
}

func Test_Duration_AcceptsAboveZeroUpTo120Seconds(t *testing.T) {
	for query, want := range map[string]time.Duration{"": 0, "?wait=30s": 30 * time.Second, "?wait=2m": 2 * time.Minute} {
		var got time.Duration
		call(t, query, func(c *gin.Context) { got, _ = Duration(c) })
		require.Equal(t, want, got, query)
	}
	for _, query := range []string{"?wait=0s", "?wait=-1s", "?wait=121s", "?wait=soon"} {
		refused := call(t, query, func(c *gin.Context) { Duration(c) })
		require.Equal(t, http.StatusBadRequest, refused.Code, query)
		require.Contains(t, refused.Body.String(), `"field":"wait"`, query)
	}
}

func Test_Read_AnswersOKOnceTheStateIsFinal(t *testing.T) {
	s := &steps{statuses: []string{"new", "new", "done"}}

	r := call(t, "", func(c *gin.Context) { Read(c, fast(NewDrain(), nil), time.Second, s.load, isDone) })

	require.Equal(t, http.StatusOK, r.Code)
	require.Equal(t, "done", statusOf(t, r))
	require.Equal(t, 3, s.reads)
}

func Test_Read_AnswersAcceptedWithRetryAfterWhenTheWaitEnds(t *testing.T) {
	s := &steps{statuses: []string{"new"}}

	r := call(t, "", func(c *gin.Context) { Read(c, fast(NewDrain(), nil), 30*time.Millisecond, s.load, isDone) })

	require.Equal(t, http.StatusAccepted, r.Code)
	require.Equal(t, RetryAfter, r.Header().Get("Retry-After"))
	require.Equal(t, "new", statusOf(t, r))
}

func Test_Read_WithoutWaitAnswersAtOnce(t *testing.T) {
	s := &steps{statuses: []string{"new"}}

	r := call(t, "", func(c *gin.Context) { Read(c, fast(NewDrain(), nil), 0, s.load, isDone) })

	require.Equal(t, http.StatusOK, r.Code)
	require.Equal(t, 1, s.reads)
}

func Test_Read_ReturnsAtOnceWhenTheServerDrains(t *testing.T) {
	drain := NewDrain()
	drain.Close()
	drain.Close()
	s := &steps{statuses: []string{"new"}}
	started := time.Now()

	r := call(t, "", func(c *gin.Context) { Read(c, Waiter{drain: drain, every: time.Second}, time.Minute, s.load, isDone) })

	require.Equal(t, http.StatusAccepted, r.Code)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func Test_Started_AnswersAcceptedWithoutWaitAndErrorsAsErrors(t *testing.T) {
	s := &steps{statuses: []string{"done"}}
	w := fast(NewDrain(), nil)

	plain := call(t, "", func(c *gin.Context) { Started(c, w, 0, thing{Status: "new"}, nil, s.load, isDone) })
	failed := call(t, "", func(c *gin.Context) { Started(c, w, time.Second, thing{}, apperror.Conflict("busy"), s.load, isDone) })
	waited := call(t, "", func(c *gin.Context) { Started(c, w, time.Second, thing{Status: "new"}, nil, s.load, isDone) })

	require.Equal(t, http.StatusAccepted, plain.Code)
	require.Equal(t, http.StatusConflict, failed.Code)
	require.Equal(t, http.StatusOK, waited.Code)
	require.Equal(t, "done", statusOf(t, waited))
}

func Test_Started_FollowsTheChangeRequestOfAGatedCall(t *testing.T) {
	pending := &approval.Pending{Kind: "change_request", ID: "c1", Status: "pending"}
	s := &steps{statuses: []string{"done"}}

	executed := call(t, "", func(c *gin.Context) {
		Started(c, fast(NewDrain(), requests{status: "executed"}), time.Second, thing{}, pending, s.load, isDone)
	})
	waiting := call(t, "", func(c *gin.Context) {
		Started(c, fast(NewDrain(), requests{status: "pending"}), 20*time.Millisecond, thing{}, pending, s.load, isDone)
	})

	require.Equal(t, http.StatusOK, executed.Code)
	require.JSONEq(t, `{"kind":"change_request","id":"c1","status":"executed","summary":"","approve_url":"","poll":"","expires_at":"0001-01-01T00:00:00Z"}`, executed.Body.String())
	require.Equal(t, http.StatusAccepted, waiting.Code)
	require.Equal(t, "pending", statusOf(t, waiting))
	require.Zero(t, s.reads)
}

func Test_Until_StopsWhenTheCallerGoesAway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &steps{statuses: []string{"new"}}
	time.AfterFunc(20*time.Millisecond, cancel)
	started := time.Now()

	last, err := Until(ctx, Waiter{drain: NewDrain(), every: 5 * time.Millisecond}, time.Minute, thing{Status: "new"}, s.load, isDone)

	require.NoError(t, err)
	require.Equal(t, "new", last.Status)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func gone(ctx context.Context) (thing, error) {
	<-ctx.Done()
	return thing{}, ctx.Err()
}

func Test_Read_WritesNothingWhenTheCallerIsGone(t *testing.T) {
	for name, load := range map[string]Load[thing]{"failed read": gone, "unfinished": (&steps{statuses: []string{"new"}}).load} {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)
		var written bool

		r := call(t, "", func(c *gin.Context) {
			c.Request = c.Request.WithContext(ctx)
			Read(c, fast(NewDrain(), nil), time.Minute, load, isDone)
			written = c.Writer.Written()
		})

		require.False(t, written, name)
		require.Zero(t, r.Body.Len(), name)
	}
}

func Test_PollThenRead_PollsCheaplyAndReadsTheAnswerOnce(t *testing.T) {
	cheap := &steps{statuses: []string{"new", "new", "done"}}
	full := &steps{statuses: []string{"done"}}

	r := call(t, "", func(c *gin.Context) {
		PollThenRead(c, fast(NewDrain(), nil), time.Second, cheap.load, full.load, isDone)
	})

	require.Equal(t, http.StatusOK, r.Code)
	require.Equal(t, 3, cheap.reads)
	require.Equal(t, 1, full.reads)
}

func Test_PollThenRead_AnswersTheFullReadWhenTheWaitEnds(t *testing.T) {
	cheap := &steps{statuses: []string{"new"}}
	full := &steps{statuses: []string{"new"}}
	unwaited := &steps{statuses: []string{"new"}}

	r := call(t, "", func(c *gin.Context) {
		PollThenRead(c, fast(NewDrain(), nil), 20*time.Millisecond, cheap.load, full.load, isDone)
	})
	plain := call(t, "", func(c *gin.Context) { PollThenRead(c, fast(NewDrain(), nil), 0, unwaited.load, full.load, isDone) })

	require.Equal(t, http.StatusAccepted, r.Code)
	require.Equal(t, RetryAfter, r.Header().Get("Retry-After"))
	require.Equal(t, http.StatusOK, plain.Code)
	require.Zero(t, unwaited.reads)
	require.Equal(t, 2, full.reads)
}

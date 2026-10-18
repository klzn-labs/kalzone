package publish_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/internal/queued/natstest"
	"github.com/klzn-labs/kalzone/internal/queued/publish"
	"github.com/klzn-labs/kalzone/internal/queued/registry"
	"github.com/stretchr/testify/require"
)

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type queues struct{ q model.Queue }

func (s queues) Lookup(name, tokenHash string) (model.Queue, registry.LookupResult) {
	if name != s.q.Name {
		return model.Queue{}, registry.NotFound
	}
	if tokenHash == hash("elsewhere") {
		return model.Queue{}, registry.Forbidden
	}
	if tokenHash != hash("tok") {
		return model.Queue{}, registry.Unauthorized
	}
	return s.q, registry.Found
}

type publisher struct {
	got  []model.Outgoing
	fail bool
}

func (p *publisher) Publish(_ context.Context, _ model.Queue, m model.Outgoing) (string, error) {
	if p.fail {
		return "", errors.New("nats down")
	}
	p.got = append(p.got, m)
	return "m1", nil
}

func post(t *testing.T, h http.Handler, path, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func setup() (http.Handler, *publisher) {
	q := natstest.Queue("jobs")
	q.Settings.MaxMessageBytes = 16
	p := &publisher{}
	return publish.New(queues{q: q}, p).Handler(), p
}

func Test_Publish_AcceptsAKnownPublisher(t *testing.T) {
	h, p := setup()

	rec := post(t, h, "/v1/queues/jobs/messages", "tok", `{"a":1}`, map[string]string{
		"Content-Type": "application/json", "Idempotency-Key": "city-7", "Platform-Delay": "60",
	})

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"message_id":"m1"}`, rec.Body.String())
	require.Equal(t, "application/json", p.got[0].ContentType)
	require.Equal(t, "city-7", p.got[0].IdempotencyKey)
	require.WithinDuration(t, time.Now().Add(time.Minute), p.got[0].Due, 2*time.Second)
}

func Test_Publish_RejectsWhatItMust(t *testing.T) {
	h, p := setup()

	cases := map[int]*httptest.ResponseRecorder{
		http.StatusUnauthorized:          post(t, h, "/v1/queues/jobs/messages", "", "x", nil),
		http.StatusForbidden:             post(t, h, "/v1/queues/jobs/messages", "elsewhere", "x", nil),
		http.StatusNotFound:              post(t, h, "/v1/queues/nope/messages", "tok", "x", nil),
		http.StatusRequestEntityTooLarge: post(t, h, "/v1/queues/jobs/messages", "tok", strings.Repeat("x", 17), nil),
		http.StatusBadRequest:            post(t, h, "/v1/queues/jobs/messages", "tok", "x", map[string]string{"Platform-Delay": "90000"}),
	}
	for want, rec := range cases {
		require.Equal(t, want, rec.Code, rec.Body.String())
	}
	require.Equal(t, http.StatusUnauthorized, post(t, h, "/v1/queues/jobs/messages", "invalid", "x", nil).Code)
	require.Empty(t, p.got)
}

func Test_Publish_BackendDownIs503(t *testing.T) {
	h, p := setup()
	p.fail = true

	rec := post(t, h, "/v1/queues/jobs/messages", "tok", "x", nil)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

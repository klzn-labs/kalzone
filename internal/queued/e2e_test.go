package queued_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/queued"
	"github.com/adrianozp/platform/internal/queued/natstest"
	"github.com/adrianozp/platform/internal/queued/signing"
	"github.com/adrianozp/platform/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

type fakeControlPlane struct {
	mu      sync.Mutex
	desired queuedapi.Desired
	stats   []queuedapi.Stats
	done    []string
}

func (f *fakeControlPlane) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer gateway-token", r.Header.Get("Authorization"))
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == queuedapi.DesiredPath:
			json.NewEncoder(w).Encode(f.desired)
		case r.URL.Path == queuedapi.StatsPath:
			var report queuedapi.StatsReport
			json.NewDecoder(r.Body).Decode(&report)
			f.stats = report.Stats
		case strings.HasSuffix(r.URL.Path, "/done"):
			f.done = append(f.done, r.URL.Path)
			f.desired.Commands = nil
		}
		w.WriteHeader(http.StatusOK)
	})
}

func (f *fakeControlPlane) read() ([]queuedapi.Stats, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats, append([]string{}, f.done...)
}

func hashOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func signedConsumer(t *testing.T) (*httptest.Server, chan string) {
	received := make(chan string, 1)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if signing.Verify(r.Header.Get("Platform-Signature"), []string{"signing-key"}, body, time.Now(), time.Minute) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		received <- string(body)
	}))
	t.Cleanup(consumer.Close)
	return consumer, received
}

func desiredFor(target string) queuedapi.Desired {
	return queuedapi.Desired{
		Queues: []queuedapi.Queue{{
			ID: "q1", Name: "place-import", Environment: "dev", Version: 1, Target: target,
			Settings: queuedapi.Settings{Path: "/hooks/place-import", RatePerSecond: 10, MaxInFlight: 2, MaxAttempts: 3,
				Backoff: queuedapi.Backoff{Kind: "fixed", Initial: "100ms", Max: "100ms"}, DeliveryTimeout: "2s", Retention: "1h", MaxMessageBytes: 4096},
			PublisherHashes: []string{hashOf("importer-token")}, SigningKey: "signing-key",
		}},
		Commands: []queuedapi.Command{{ID: "c1", QueueID: "q1", Kind: "purge_dead"}},
	}
}

func startGateway(t *testing.T, platformURL string) *httptest.Server {
	cfg := queued.Config{PlatformURL: platformURL, Token: "gateway-token", Poll: 50 * time.Millisecond, StatsEvery: 100 * time.Millisecond}
	app, err := queued.Build(cfg, natstest.Start(t))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	go app.Run(ctx)
	gateway := httptest.NewServer(app.Handler())
	t.Cleanup(func() {
		gateway.Close()
		cancel()
		app.Stop()
	})
	return gateway
}

func publishes(gatewayURL, token, body string) func() bool {
	return func() bool {
		req, _ := http.NewRequest(http.MethodPost, gatewayURL+"/v1/queues/place-import/messages", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		return err == nil && resp.StatusCode == http.StatusAccepted
	}
}

func Test_Gateway_PublishDeliverReportAndCommand(t *testing.T) {
	consumer, received := signedConsumer(t)
	cp := &fakeControlPlane{desired: desiredFor(consumer.URL + "/hooks/place-import")}
	platform := httptest.NewServer(cp.handler(t))
	defer platform.Close()
	gateway := startGateway(t, platform.URL)

	require.Eventually(t, publishes(gateway.URL, "importer-token", `{"city":7}`), 5*time.Second, 50*time.Millisecond)

	select {
	case body := <-received:
		require.Equal(t, `{"city":7}`, body)
	case <-time.After(5 * time.Second):
		t.Fatal("the consumer never received the message")
	}
	require.Eventually(t, func() bool {
		stats, done := cp.read()
		return len(stats) == 1 && stats[0].DeliveredPerMinute >= 1 && stats[0].Depth == 0 && len(done) == 1
	}, 5*time.Second, 50*time.Millisecond)
}

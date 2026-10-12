package discord_test

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/approvals/repositories/discord"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/stretchr/testify/require"
)

type received struct {
	Content         string `json:"content"`
	AllowedMentions struct {
		Parse *[]string `json:"parse"`
	} `json:"allowed_mentions"`
}

func webhook(t *testing.T, status int, release chan struct{}) (*httptest.Server, chan received) {
	t.Helper()
	got := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m received
		_ = json.NewDecoder(r.Body).Decode(&m)
		<-release
		got <- m
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func Test_Notify_PostsWithoutMentionsAndWithoutWaiting(t *testing.T) {
	release := make(chan struct{})
	server, got := webhook(t, http.StatusNoContent, release)
	n := discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: server.URL}})

	started := time.Now()
	n.Notify("bob via claude-local asks to deploy release #2 to prod")
	returnedIn := time.Since(started)
	close(release)

	m := <-got
	require.Less(t, returnedIn, 100*time.Millisecond)
	require.Equal(t, "bob via claude-local asks to deploy release #2 to prod", m.Content)
	require.NotNil(t, m.AllowedMentions.Parse)
	require.Empty(t, *m.AllowedMentions.Parse)
}

func Test_Notify_IsQuietWithoutAURLAndSurvivesAFailingWebhook(t *testing.T) {
	captured := capturedLog(t)
	release := make(chan struct{})
	close(release)
	server, got := webhook(t, http.StatusInternalServerError, release)

	discord.NewNotifier(config.Config{}).Notify("nobody listens")
	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: server.URL}}).Notify("fails")
	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: "http://127.0.0.1:1/hook"}}).Notify("unreachable")

	require.Equal(t, "fails", (<-got).Content)
	require.Eventually(t, func() bool { return logLines(captured) >= 2 }, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, 2, logLines(captured))
	require.Empty(t, got)
}

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func capturedLog(t *testing.T) *logBuffer {
	t.Helper()
	captured := &logBuffer{}
	previous := log.Writer()
	log.SetOutput(captured)
	t.Cleanup(func() { log.SetOutput(previous) })
	return captured
}

func logLines(captured *logBuffer) int {
	return strings.Count(captured.String(), "approval notification not sent")
}

func Test_Notify_LogsFailuresWithoutTheWebhookURL(t *testing.T) {
	captured := capturedLog(t)
	release := make(chan struct{})
	close(release)
	server, _ := webhook(t, http.StatusInternalServerError, release)
	failing := server.URL + "/api/webhooks/42/failing-secret"
	unreachable := "http://127.0.0.1:1/api/webhooks/42/unreachable-secret"

	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: failing}}).Notify("fails")
	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: unreachable}}).Notify("unreachable")

	require.Eventually(t, func() bool { return logLines(captured) == 2 }, 5*time.Second, 10*time.Millisecond)
	require.NotContains(t, captured.String(), "/api/webhooks")
	require.NotContains(t, captured.String(), "secret")
	require.NotContains(t, captured.String(), server.URL)
}

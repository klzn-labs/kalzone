package cron

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/signing"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

const errorBodyLimit = 512

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type sender struct {
	client HTTPDoer
}

type sendResult struct {
	status int
	header http.Header
	body   string
	err    error
}

func (r sendResult) describe() string {
	if r.err != nil {
		return r.err.Error()
	}
	return fmt.Sprintf("HTTP %d: %s", r.status, r.body)
}

func (s sender) send(ctx context.Context, c Cron, run queuedapi.CronRun) sendResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Target, bytes.NewReader(c.Payload))
	if err != nil {
		return sendResult{err: err}
	}
	setHeaders(req.Header, c, run)
	resp, err := s.client.Do(req)
	if err != nil {
		return sendResult{err: err}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
	return sendResult{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func setHeaders(h http.Header, c Cron, run queuedapi.CronRun) {
	h.Set("Content-Type", "application/json")
	h.Set("Platform-Cron", c.Name)
	h.Set("Platform-Scheduled-At", run.ScheduledAt.UTC().Format(time.RFC3339))
	h.Set("Platform-Run-Id", run.ID)
	h.Set("Platform-Attempt", strconv.Itoa(run.Attempts))
	h.Set("Platform-Signature", signing.Sign(c.SigningKey, time.Now(), c.Payload))
}

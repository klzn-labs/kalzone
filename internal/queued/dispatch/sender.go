package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/internal/queued/signing"
)

const errorBodyLimit = 512

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type sender struct {
	client httpDoer
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

func (s sender) send(ctx context.Context, q model.Queue, m model.Delivery) sendResult {
	ctx, cancel := context.WithTimeout(ctx, q.Settings.DeliveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.Target, bytes.NewReader(m.Body))
	if err != nil {
		return sendResult{err: err}
	}
	setHeaders(req.Header, q, m)
	resp, err := s.client.Do(req)
	if err != nil {
		return sendResult{err: err}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
	return sendResult{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func setHeaders(h http.Header, q model.Queue, m model.Delivery) {
	h.Set("Content-Type", m.ContentType)
	h.Set("Platform-Message-Id", m.ID)
	h.Set("Platform-Attempt", strconv.Itoa(m.Attempt))
	h.Set("Platform-Queue", q.Name)
	h.Set("Platform-Published-At", m.PublishedAt.UTC().Format(time.RFC3339))
	h.Set("Platform-Signature", signing.Sign(q.SigningKey, time.Now(), m.Body))
}

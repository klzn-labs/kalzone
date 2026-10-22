package upstream

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const answerLimit = 64 << 20

var errUnreachable = apperror.New(apperror.CodeUpstream, "the telemetry store did not answer").WithHint("try again in a minute")

type Client struct {
	prometheus string
	loki       string
	http       *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{
		prometheus: strings.TrimRight(cfg.Telemetry.PrometheusURL, "/"), loki: strings.TrimRight(cfg.Telemetry.LokiURL, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c Client) Prometheus(ctx context.Context, path string, form url.Values) (entities.TelemetryAnswer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.prometheus+path, strings.NewReader(form.Encode()))
	if err != nil {
		return entities.TelemetryAnswer{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c Client) Loki(ctx context.Context, call entities.LokiCall) (entities.TelemetryAnswer, error) {
	target := c.loki + call.Path
	if call.RawQuery != "" {
		target += "?" + call.RawQuery
	}
	req, err := http.NewRequestWithContext(ctx, call.Method, target, bytes.NewReader(call.Body))
	if err != nil {
		return entities.TelemetryAnswer{}, err
	}
	if call.ContentType != "" {
		req.Header.Set("Content-Type", call.ContentType)
	}
	req.Header.Set("X-Scope-OrgID", strings.Join(call.Tenants, "|"))
	return c.do(req)
}

func (c Client) do(req *http.Request) (entities.TelemetryAnswer, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return entities.TelemetryAnswer{}, errUnreachable.WithDetails(map[string]any{"cause": err.Error()})
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, answerLimit))
	if err != nil {
		return entities.TelemetryAnswer{}, errUnreachable.WithDetails(map[string]any{"cause": err.Error()})
	}
	return entities.TelemetryAnswer{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
}

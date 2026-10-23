package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string, client *http.Client) Client {
	return Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: client}
}

func (c Client) Desired(ctx context.Context) (queuedapi.Desired, error) {
	var desired queuedapi.Desired
	err := c.call(ctx, http.MethodGet, queuedapi.DesiredPath, nil, &desired)
	return desired, err
}

func (c Client) Report(ctx context.Context, stats []queuedapi.Stats) error {
	return c.call(ctx, http.MethodPost, queuedapi.StatsPath, queuedapi.StatsReport{Stats: stats}, nil)
}

func (c Client) Done(ctx context.Context, commandID string) error {
	return c.call(ctx, http.MethodPost, queuedapi.DonePath(commandID), nil, nil)
}

func (c Client) ReportRuns(ctx context.Context, runs []queuedapi.CronRun) error {
	return c.call(ctx, http.MethodPost, queuedapi.CronRunsPath, queuedapi.CronRunReport{Runs: runs}, nil)
}

func (c Client) call(ctx context.Context, method, path string, body, out any) error {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("control plane answered %d to %s %s: %s", resp.StatusCode, method, path, detail)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

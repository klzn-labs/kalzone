package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/agentapi"
)

type Client struct {
	baseURL string
	token   string
	name    string
	version string
	http    *http.Client
}

func NewClient(cfg Config) Client {
	return Client{
		baseURL: strings.TrimRight(cfg.PlatformURL, "/"), token: cfg.Token, name: cfg.Name, version: cfg.Version,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c Client) Claim(ctx context.Context) (agentapi.Workload, bool, error) {
	var w agentapi.Workload
	status, err := c.call(ctx, http.MethodPost, agentapi.ClaimPath, nil, &w)
	return w, status == http.StatusOK, err
}

func (c Client) Events(ctx context.Context, deploymentID string, lines []string) error {
	_, err := c.call(ctx, http.MethodPost, agentapi.EventsPath(deploymentID), agentapi.Events{Lines: lines}, nil)
	return err
}

func (c Client) Finish(ctx context.Context, deploymentID string, succeeded bool, message string, failure *agentapi.Failure) error {
	body := agentapi.Finish{Succeeded: succeeded, Message: message, Failure: failure}
	_, err := c.call(ctx, http.MethodPost, agentapi.FinishPath(deploymentID), body, nil)
	return err
}

func (c Client) Status(ctx context.Context, snapshots []agentapi.Snapshot) error {
	_, err := c.call(ctx, http.MethodPost, agentapi.StatusPath, agentapi.StatusReport{Workloads: snapshots}, nil)
	return err
}

func (c Client) Desired(ctx context.Context) ([]agentapi.Desired, error) {
	var list agentapi.DesiredList
	_, err := c.call(ctx, http.MethodGet, agentapi.DesiredPath, nil, &list)
	return list.Workloads, err
}

func (c Client) Drift(ctx context.Context, d agentapi.Drift) error {
	_, err := c.call(ctx, http.MethodPost, agentapi.DriftPath, d, nil)
	return err
}

func (c Client) call(ctx context.Context, method, path string, body, out any) (int, error) {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
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
	req.Header.Set(agentapi.NameHeader, c.name)
	req.Header.Set(agentapi.VersionHeader, c.version)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

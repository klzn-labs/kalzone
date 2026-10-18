package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/config"
)

type Client struct {
	url   string
	token string
	http  *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{url: strings.TrimRight(cfg.Grafana.URL, "/"), token: cfg.Grafana.Token, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c Client) Enabled() bool {
	return c.url != "" && c.token != ""
}

func (c Client) EnsureFolder(ctx context.Context) error {
	status, err := c.call(ctx, http.MethodGet, "/api/folders/"+entities.FolderUID, nil)
	if err != nil || status == http.StatusOK {
		return err
	}
	body := map[string]string{"uid": entities.FolderUID, "title": entities.FolderTitle}
	status, err = c.call(ctx, http.MethodPost, "/api/folders", body)
	return expect(status, err, http.StatusOK)
}

func (c Client) UpsertDashboard(ctx context.Context, dashboard map[string]any) error {
	body := map[string]any{"dashboard": dashboard, "folderUid": entities.FolderUID, "overwrite": true, "message": "written by the platform"}
	status, err := c.call(ctx, http.MethodPost, "/api/dashboards/db", body)
	return expect(status, err, http.StatusOK)
}

func (c Client) DeleteDashboard(ctx context.Context, uid string) error {
	status, err := c.call(ctx, http.MethodDelete, "/api/dashboards/uid/"+url.PathEscape(uid), nil)
	if status == http.StatusNotFound {
		return nil
	}
	return expect(status, err, http.StatusOK)
}

func expect(status int, err error, want int) error {
	if err != nil || status == want {
		return err
	}
	return fmt.Errorf("grafana answered %d", status)
}

func (c Client) call(ctx context.Context, method, path string, body any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

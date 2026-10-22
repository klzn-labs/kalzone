package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/config"
)

type Client struct {
	url      string
	user     string
	password string
	http     *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{
		url: strings.TrimRight(cfg.Grafana.URL, "/"), user: cfg.Grafana.AdminUser, password: cfg.Grafana.AdminPassword,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c Client) Enabled() bool {
	return c.url != "" && c.user != "" && c.password != ""
}

func (c Client) AdminLogin() string {
	return c.user
}

func (c Client) EnsureFolder(ctx context.Context, org int) error {
	status, err := c.call(ctx, org, http.MethodGet, "/api/folders/"+entities.FolderUID, nil, nil)
	if err != nil || status == http.StatusOK {
		return err
	}
	body := map[string]string{"uid": entities.FolderUID, "title": entities.FolderTitle}
	status, err = c.call(ctx, org, http.MethodPost, "/api/folders", body, nil)
	return expect(status, err, http.StatusOK)
}

func (c Client) UpsertDashboard(ctx context.Context, org int, dashboard map[string]any) error {
	body := map[string]any{"dashboard": dashboard, "folderUid": entities.FolderUID, "overwrite": true, "message": "written by the platform"}
	status, err := c.call(ctx, org, http.MethodPost, "/api/dashboards/db", body, nil)
	return expect(status, err, http.StatusOK)
}

func (c Client) DeleteDashboard(ctx context.Context, org int, uid string) error {
	status, err := c.call(ctx, org, http.MethodDelete, "/api/dashboards/uid/"+url.PathEscape(uid), nil, nil)
	if status == http.StatusNotFound {
		return nil
	}
	return expect(status, err, http.StatusOK)
}

func (c Client) DashboardUIDs(ctx context.Context, org int) ([]string, error) {
	var found []struct {
		UID string `json:"uid"`
	}
	path := "/api/search?type=dash-db&limit=5000&folderUIDs=" + entities.FolderUID
	status, err := c.call(ctx, org, http.MethodGet, path, nil, &found)
	if err := expect(status, err, http.StatusOK); err != nil {
		return nil, err
	}
	uids := make([]string, 0, len(found))
	for _, d := range found {
		uids = append(uids, d.UID)
	}
	return uids, nil
}

func expect(status int, err error, want int) error {
	if err != nil || status == want {
		return err
	}
	return fmt.Errorf("grafana answered %d", status)
}

func (c Client) call(ctx context.Context, org int, method, path string, body, out any) (int, error) {
	req, err := c.request(ctx, org, method, path, body)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out == nil || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
}

func (c Client) request(ctx context.Context, org int, method, path string, body any) (*http.Request, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Grafana-Org-Id", strconv.Itoa(org))
	return req, nil
}

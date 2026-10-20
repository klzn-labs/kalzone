package pushgateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const attachmentsJob = "kalzone_databases"

type Client struct {
	url  string
	http *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{url: strings.TrimRight(cfg.Metrics.PushgatewayURL, "/"), http: &http.Client{Timeout: 10 * time.Second}}
}

func (c Client) Enabled() bool {
	return c.url != ""
}

func (c Client) PublishAttachments(ctx context.Context, services []entities.Service) error {
	body := attachmentSeries(services)
	if body == "" {
		return c.send(ctx, http.MethodDelete, "")
	}
	return c.send(ctx, http.MethodPut, body)
}

func attachmentSeries(services []entities.Service) string {
	var body strings.Builder
	for _, s := range services {
		for _, db := range s.Databases {
			fmt.Fprintf(&body, "kalzone_database_attachment{service=%s,environment=%s,schema=%s,mysql_user=%s} %d\n",
				strconv.Quote(s.Slug), strconv.Quote(db.Environment), strconv.Quote(db.Schema), strconv.Quote(db.User), db.MaxConnections)
		}
	}
	return body.String()
}

func (c Client) send(ctx context.Context, method, body string) error {
	req, err := http.NewRequestWithContext(ctx, method, c.url+"/metrics/job/"+attachmentsJob, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("pushgateway answered %d", resp.StatusCode)
	}
	return nil
}

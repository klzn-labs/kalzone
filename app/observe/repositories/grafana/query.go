package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var (
	errNotConfigured = apperror.New(apperror.CodeNotConfigured, "Grafana is not configured on this platform").
				WithHint("set PLATFORM_GRAFANA_URL and PLATFORM_GRAFANA_TOKEN")
	errUpstream = apperror.New(apperror.CodeUpstream, "Grafana did not answer the query").WithHint("try again in a minute")
)

type vector struct {
	Data struct {
		Result []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type streams struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (c Client) Instant(ctx context.Context, uid, query string) (float64, bool, error) {
	var answer vector
	if err := c.query(ctx, "/api/datasources/proxy/uid/"+uid+"/api/v1/query", url.Values{"query": {query}}, &answer); err != nil {
		return 0, false, err
	}
	if len(answer.Data.Result) == 0 {
		return 0, false, nil
	}
	raw, _ := answer.Data.Result[0].Value[1].(string)
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil, nil
}

func (c Client) LogLines(ctx context.Context, uid, query string, start, end time.Time, limit int) ([]entities.LogLine, error) {
	params := url.Values{
		"query": {query}, "start": {nanos(start)}, "end": {nanos(end)},
		"limit": {strconv.Itoa(limit)}, "direction": {"backward"},
	}
	var answer streams
	if err := c.query(ctx, "/api/datasources/proxy/uid/"+uid+"/loki/api/v1/query_range", params, &answer); err != nil {
		return nil, err
	}
	return flatten(answer), nil
}

func nanos(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}

func flatten(answer streams) []entities.LogLine {
	lines := []entities.LogLine{}
	for _, stream := range answer.Data.Result {
		for _, value := range stream.Values {
			lines = append(lines, entities.LogLine{At: parseNanos(value[0]), Container: stream.Stream["container"], Line: value[1]})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At.Before(lines[j].At) })
	return lines
}

func parseNanos(raw string) time.Time {
	n, _ := strconv.ParseInt(raw, 10, 64)
	return time.Unix(0, n).UTC()
}

func (c Client) query(ctx context.Context, path string, params url.Values, out any) error {
	if !c.Enabled() {
		return errNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return errUpstream.WithDetails(map[string]any{"cause": err.Error()})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errUpstream.WithDetails(map[string]any{"status": resp.StatusCode})
	}
	return decode(json.NewDecoder(resp.Body).Decode(out))
}

func decode(err error) error {
	if err != nil {
		return errUpstream.WithDetails(map[string]any{"cause": fmt.Sprint(err)})
	}
	return nil
}

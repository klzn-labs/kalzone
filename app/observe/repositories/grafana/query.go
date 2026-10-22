package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
				WithHint("set PLATFORM_GRAFANA_URL, PLATFORM_GRAFANA_ADMIN_USER and PLATFORM_GRAFANA_ADMIN_PASSWORD")
	errUpstream = apperror.New(apperror.CodeUpstream, "Grafana did not answer the query").WithHint("try again in a minute")
)

type vector struct {
	Data struct {
		Result []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type matrix struct {
	Data struct {
		Result []struct {
			Values [][2]any `json:"values"`
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

func (c Client) Range(ctx context.Context, uid, query string, start, end time.Time, step time.Duration) ([]entities.Sample, error) {
	params := url.Values{
		"query": {query}, "start": {seconds(start)}, "end": {seconds(end)}, "step": {strconv.Itoa(int(step.Seconds()))},
	}
	var answer matrix
	if err := c.query(ctx, "/api/datasources/proxy/uid/"+uid+"/api/v1/query_range", params, &answer); err != nil {
		return nil, err
	}
	if len(answer.Data.Result) == 0 {
		return []entities.Sample{}, nil
	}
	return samplesOf(answer.Data.Result[0].Values), nil
}

func samplesOf(values [][2]any) []entities.Sample {
	samples := make([]entities.Sample, 0, len(values))
	for _, v := range values {
		at, _ := v[0].(float64)
		raw, _ := v[1].(string)
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsNaN(value) {
			samples = append(samples, entities.Sample{At: time.Unix(int64(at), 0).UTC(), Value: value})
		}
	}
	return samples
}

func seconds(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
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
	req, err := c.request(ctx, entities.MainOrg, http.MethodGet, path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
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

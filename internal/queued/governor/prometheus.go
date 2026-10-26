package governor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var errNoData = errors.New("governor: the query returned no single series")

type Prometheus struct {
	URL    string
	Query  string
	Client *http.Client
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p Prometheus) Read(ctx context.Context) (Sample, error) {
	endpoint := p.URL + "/api/v1/query?" + url.Values{"query": {p.Query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Sample{}, err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return Sample{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Sample{}, fmt.Errorf("governor: prometheus answered %d", resp.StatusCode)
	}
	var body queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Sample{}, err
	}
	return body.sample()
}

func (r queryResponse) sample() (Sample, error) {
	if r.Status != "success" || len(r.Data.Result) != 1 {
		return Sample{}, errNoData
	}
	at, okAt := r.Data.Result[0].Value[0].(float64)
	raw, okValue := r.Data.Result[0].Value[1].(string)
	if !okAt || !okValue {
		return Sample{}, errNoData
	}
	value, err := strconv.ParseFloat(raw, 64)
	seconds := int64(at)
	return Sample{Value: value, At: time.Unix(seconds, int64((at-float64(seconds))*1e9))}, err
}

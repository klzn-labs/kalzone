package queued

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/governor"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/prometheus/client_golang/prometheus"
)

const defaultAdaptiveQuery = `sum(rate(container_cpu_usage_seconds_total{container_label_com_docker_compose_service="mysql"}[1m]))*100`

var errAdaptiveLoop = errors.New("queued: QUEUED_ADAPTIVE_PERIOD must be positive and QUEUED_ADAPTIVE_STALE_AFTER at least 1")

type AdaptiveConfig struct {
	PrometheusURL string
	Query         string
	Governor      governor.Settings
}

func loadAdaptive() (AdaptiveConfig, error) {
	setpoint, setpointErr := envFloat("QUEUED_ADAPTIVE_SETPOINT", "50")
	kp, kpErr := envFloat("QUEUED_ADAPTIVE_KP", "0.005")
	ki, kiErr := envFloat("QUEUED_ADAPTIVE_KI", "0.0002")
	staleAfter, staleErr := strconv.Atoi(env("QUEUED_ADAPTIVE_STALE_AFTER", "3"))
	filterSize, filterErr := strconv.Atoi(env("QUEUED_ADAPTIVE_FILTER_SIZE", "1"))
	period, periodErr := time.ParseDuration(env("QUEUED_ADAPTIVE_PERIOD", "15s"))
	cfg := AdaptiveConfig{
		PrometheusURL: os.Getenv("QUEUED_ADAPTIVE_PROMETHEUS_URL"), Query: env("QUEUED_ADAPTIVE_QUERY", defaultAdaptiveQuery),
		Governor: governor.Settings{Setpoint: setpoint, Kp: kp, Ki: ki, Period: period, StaleAfter: staleAfter, FilterSize: filterSize},
	}
	err := errors.Join(setpointErr, kpErr, kiErr, staleErr, filterErr, periodErr)
	if err == nil && (period <= 0 || staleAfter < 1) {
		err = errAdaptiveLoop
	}
	return cfg, err
}

func envFloat(key, fallback string) (float64, error) {
	return strconv.ParseFloat(env(key, fallback), 64)
}

func (c AdaptiveConfig) build(registerer prometheus.Registerer) (*governor.Governor, error) {
	if c.PrometheusURL == "" {
		return nil, nil
	}
	reader := governor.Prometheus{URL: c.PrometheusURL, Query: c.Query, Client: &http.Client{Timeout: c.Governor.Period / 2}}
	return governor.New(reader, c.Governor, time.Now, registerer)
}

func throttleOf(g *governor.Governor) model.Throttle {
	if g == nil {
		return model.FixedThrottle{}
	}
	return g
}

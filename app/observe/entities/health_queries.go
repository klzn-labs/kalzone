package entities

import (
	"fmt"
	"time"
)

const (
	HealthWindow = "5m"
	SustainFor   = 5 * time.Minute
)

func SustainedQuery(query string) string {
	return fmt.Sprintf(`min_over_time((%s)[%s:1m])`, query, HealthWindow)
}

func ResponseQuery(slug string) string {
	return fmt.Sprintf(`histogram_quantile(0.95, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{service=~"%s-prod-(blue|green)@file"}[%s]))) * 1000`, slug, HealthWindow)
}

func CPUQuery(slug string, cpuMillis int) string {
	return fmt.Sprintf(`max(rate(container_cpu_usage_seconds_total{name=~"%s-prod-(blue|green)"}[%s])) * 100000 / %d`, slug, HealthWindow, cpuMillis)
}

func DBCPUQuery() string {
	return fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{name="%s"}[%s])) * 100`, mysqlContainer, HealthWindow)
}

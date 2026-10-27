package httpserver

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "platform_http_request_duration_seconds",
	Help:    "Duration of HTTP requests served by the control plane.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "route", "status"})

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		method, route := requestLabels(c.Request.Method, c.FullPath())
		requestDuration.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Observe(time.Since(start).Seconds())
	}
}

func requestLabels(method, route string) (string, string) {
	if route == "" {
		return "ANY", "unmatched"
	}
	return method, route
}

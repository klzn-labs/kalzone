package queued

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	PlatformURL  string
	Token        string
	NATSURL      string
	NATSUser     string
	NATSPassword string
	Addr         string
	MetricsAddr  string
	Poll         time.Duration
	StatsEvery   time.Duration
}

var errMissing = errors.New("queued: QUEUED_PLATFORM_URL, QUEUED_TOKEN and QUEUED_NATS_URL are required")

func LoadConfig() (Config, error) {
	cfg := Config{
		PlatformURL: os.Getenv("QUEUED_PLATFORM_URL"), Token: os.Getenv("QUEUED_TOKEN"),
		NATSURL: os.Getenv("QUEUED_NATS_URL"), NATSUser: os.Getenv("QUEUED_NATS_USER"), NATSPassword: os.Getenv("QUEUED_NATS_PASSWORD"),
		Addr: env("QUEUED_ADDR", ":8080"), MetricsAddr: env("QUEUED_METRICS_ADDR", ":9090"),
	}
	var pollErr, statsErr error
	cfg.Poll, pollErr = time.ParseDuration(env("QUEUED_POLL", "5s"))
	cfg.StatsEvery, statsErr = time.ParseDuration(env("QUEUED_STATS_EVERY", "15s"))
	if cfg.PlatformURL == "" || cfg.Token == "" || cfg.NATSURL == "" {
		return Config{}, errMissing
	}
	return cfg, errors.Join(pollErr, statsErr)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

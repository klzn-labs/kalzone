package agent

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	PlatformURL string
	Token       string
	Name        string
	Version     string
	Root        string
	Deployctl   string
	Docker      string
	Poll        time.Duration
	DriftEvery  time.Duration
}

var errMissing = errors.New("agent: AGENT_PLATFORM_URL and AGENT_TOKEN are required")

func LoadConfig(version string) (Config, error) {
	hostname, _ := os.Hostname()
	cfg := Config{
		PlatformURL: os.Getenv("AGENT_PLATFORM_URL"), Token: os.Getenv("AGENT_TOKEN"),
		Name: env("AGENT_NAME", strings.ToLower(hostname)), Version: version,
		Root: env("AGENT_ROOT", "/opt/plateful"), Deployctl: env("AGENT_DEPLOYCTL", "/opt/plateful/bin/deployctl"),
		Docker: env("AGENT_DOCKER", "docker"),
	}
	var pollErr, driftErr error
	cfg.Poll, pollErr = time.ParseDuration(env("AGENT_POLL", "5s"))
	cfg.DriftEvery, driftErr = time.ParseDuration(env("AGENT_DRIFT_EVERY", "60s"))
	if cfg.PlatformURL == "" || cfg.Token == "" {
		return Config{}, errMissing
	}
	return cfg, errors.Join(pollErr, driftErr)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

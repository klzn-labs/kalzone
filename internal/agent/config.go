package agent

import (
	"errors"
	"fmt"
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
	StatusEvery time.Duration

	InternalEntrypoints map[string]string
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
	var pollErr, driftErr, statusErr, entrypointsErr error
	cfg.Poll, pollErr = time.ParseDuration(env("AGENT_POLL", "5s"))
	cfg.DriftEvery, driftErr = time.ParseDuration(env("AGENT_DRIFT_EVERY", "60s"))
	cfg.StatusEvery, statusErr = time.ParseDuration(env("AGENT_STATUS_EVERY", "15s"))
	cfg.InternalEntrypoints, entrypointsErr = parseEntrypoints(os.Getenv("AGENT_INTERNAL_ENTRYPOINTS"))
	if cfg.PlatformURL == "" || cfg.Token == "" {
		return Config{}, errMissing
	}
	return cfg, errors.Join(pollErr, driftErr, statusErr, entrypointsErr)
}

func parseEntrypoints(raw string) (map[string]string, error) {
	result := map[string]string{}
	for _, pair := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' }) {
		environment, entrypoint, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || environment == "" || entrypoint == "" {
			return nil, fmt.Errorf("agent: AGENT_INTERNAL_ENTRYPOINTS wants env=entrypoint pairs, got %q", pair)
		}
		result[environment] = entrypoint
	}
	return result, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

const (
	BackoffFixed       = "fixed"
	BackoffExponential = "exponential"
)

type Backoff struct {
	Kind    string
	Initial time.Duration
	Max     time.Duration
}

type Settings struct {
	Path             string
	RatePerSecond    float64
	Adaptive         bool
	MinRatePerSecond float64
	MaxInFlight      int
	MaxAttempts      int
	Backoff          Backoff
	DeliveryTimeout  time.Duration
	Retention        time.Duration
	MaxMessageBytes  int
}

type Queue struct {
	ID              string
	Name            string
	Environment     string
	Version         int
	Settings        Settings
	Paused          bool
	Target          string
	PublisherHashes []string
	SigningKey      string
	PreviousKey     string
}

func FromAPI(q queuedapi.Queue) (Queue, error) {
	settings, err := settingsFromAPI(q.Settings)
	return Queue{
		ID: q.ID, Name: q.Name, Environment: q.Environment, Version: q.Version, Settings: settings, Paused: q.Paused,
		Target: q.Target, PublisherHashes: q.PublisherHashes, SigningKey: q.SigningKey, PreviousKey: q.PreviousKey,
	}, err
}

func settingsFromAPI(s queuedapi.Settings) (Settings, error) {
	parsed := map[string]time.Duration{}
	fields := map[string]string{"backoff.initial": s.Backoff.Initial, "backoff.max": s.Backoff.Max, "delivery_timeout": s.DeliveryTimeout, "retention": s.Retention}
	for name, raw := range fields {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Settings{}, fmt.Errorf("queue setting %s: %w", name, err)
		}
		parsed[name] = d
	}
	return Settings{
		Path: s.Path, RatePerSecond: s.RatePerSecond, Adaptive: s.Adaptive, MinRatePerSecond: s.MinRatePerSecond,
		MaxInFlight: s.MaxInFlight, MaxAttempts: s.MaxAttempts,
		Backoff:         Backoff{Kind: s.Backoff.Kind, Initial: parsed["backoff.initial"], Max: parsed["backoff.max"]},
		DeliveryTimeout: parsed["delivery_timeout"], Retention: parsed["retention"], MaxMessageBytes: s.MaxMessageBytes,
	}, nil
}

func (q Queue) Keys() []string {
	return []string{q.SigningKey, q.PreviousKey}
}

func (q Queue) token() string {
	return strings.ToUpper(strings.ReplaceAll(q.Environment+"_"+q.Name, "-", "_"))
}

func (q Queue) Stream() string       { return "Q_" + q.token() }
func (q Queue) DeadStream() string   { return "DLQ_" + q.token() }
func (q Queue) DelayStream() string  { return "QD_" + q.token() }
func (q Queue) Subject() string      { return "q." + q.Environment + "." + q.Name }
func (q Queue) DeadSubject() string  { return "dlq." + q.Environment + "." + q.Name }
func (q Queue) DelaySubject() string { return "qd." + q.Environment + "." + q.Name }

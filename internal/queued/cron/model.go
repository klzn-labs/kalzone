package cron

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/klzn-labs/kalzone/pkg/cronspec"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

const (
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunTimedOut  = "timed_out"
	RunSkipped   = "skipped"
)

type Cron struct {
	ID          string
	Name        string
	Environment string
	Schedule    cronspec.Schedule
	Target      string
	Payload     []byte
	Timeout     time.Duration
	Paused      bool
	SigningKey  string
}

func FromAPI(c queuedapi.Cron) (Cron, error) {
	schedule, err := cronspec.Parse(c.Schedule)
	if err != nil {
		return Cron{}, fmt.Errorf("cron %s/%s schedule %q: %w", c.Environment, c.Name, c.Schedule, err)
	}
	timeout, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return Cron{}, fmt.Errorf("cron %s/%s timeout: %w", c.Environment, c.Name, err)
	}
	return Cron{
		ID: c.ID, Name: c.Name, Environment: c.Environment, Schedule: schedule, Target: c.Target,
		Payload: c.Payload, Timeout: timeout, Paused: c.Paused, SigningKey: c.SigningKey,
	}, nil
}

func (c Cron) same(other Cron) bool {
	return c.Schedule.String() == other.Schedule.String() && c.Target == other.Target && string(c.Payload) == string(other.Payload) &&
		c.Timeout == other.Timeout && c.Paused == other.Paused && c.SigningKey == other.SigningKey && c.Name == other.Name
}

func ScheduledRunID(cronID string, scheduled time.Time) string {
	sum := sha256.Sum256([]byte(cronID + "|" + scheduled.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:])
}

func scheduledRun(c Cron, scheduled time.Time) queuedapi.CronRun {
	return queuedapi.CronRun{ID: ScheduledRunID(c.ID, scheduled), CronID: c.ID, ScheduledAt: scheduled.UTC()}
}

func manualRun(c Cron, commandID string, at time.Time) queuedapi.CronRun {
	return queuedapi.CronRun{ID: commandID, CronID: c.ID, Manual: true, ScheduledAt: at.UTC()}
}

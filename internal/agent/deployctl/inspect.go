package deployctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

const (
	healthEntries = 3
	mebibyte      = 1 << 20
)

type inspected struct {
	Name    string    `json:"Name"`
	Created time.Time `json:"Created"`
	Config  struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status    string    `json:"Status"`
		OOMKilled bool      `json:"OOMKilled"`
		ExitCode  int       `json:"ExitCode"`
		StartedAt time.Time `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
			Log    []struct {
				Output string `json:"Output"`
			} `json:"Log"`
		} `json:"Health"`
	} `json:"State"`
	RestartCount int `json:"RestartCount"`
	HostConfig   struct {
		Memory   int64 `json:"Memory"`
		NanoCpus int64 `json:"NanoCpus"`
	} `json:"HostConfig"`
}

func (r Runtime) Inspect(ctx context.Context, container string) (agentapi.Container, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.Docker, "inspect", container)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && strings.Contains(stderr.String(), "No such object") {
		return agentapi.Container{}, nil
	}
	if err != nil {
		return agentapi.Container{}, fmt.Errorf("docker inspect %s: %w: %s", container, err, strings.TrimSpace(stderr.String()))
	}
	var found []inspected
	if err := json.Unmarshal(out, &found); err != nil || len(found) == 0 {
		return agentapi.Container{}, errors.Join(err, fmt.Errorf("docker inspect %s answered nothing", container))
	}
	return toContainer(found[0]), nil
}

func toContainer(i inspected) agentapi.Container {
	c := agentapi.Container{
		Found: true, Name: strings.TrimPrefix(i.Name, "/"), Image: i.Config.Image, State: i.State.Status,
		ExitCode: i.State.ExitCode, OOMKilled: i.State.OOMKilled, Restarts: i.RestartCount,
		StartedAt: i.State.StartedAt, CreatedAt: i.Created,
		MemoryLimitMB: int(i.HostConfig.Memory / mebibyte), CPULimitMillis: int(i.HostConfig.NanoCpus / 1_000_000),
		HealthLog: []string{},
	}
	if i.State.Health != nil {
		c.Health, c.HealthLog = i.State.Health.Status, healthLog(i)
	}
	return c
}

func healthLog(i inspected) []string {
	entries := i.State.Health.Log
	if len(entries) > healthEntries {
		entries = entries[len(entries)-healthEntries:]
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, strings.TrimSpace(e.Output))
	}
	return lines
}

func (r Runtime) Logs(ctx context.Context, container string, lines int) ([]string, error) {
	out, err := exec.CommandContext(ctx, r.Docker, "logs", "--tail", fmt.Sprint(lines), "--timestamps", container).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker logs %s: %w", container, err)
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

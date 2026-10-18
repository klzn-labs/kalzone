package agent

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"time"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

const (
	failureLogLines = 50
	evidenceTimeout = 15 * time.Second
)

var stepClasses = map[int]string{11: "smoke_failed", 12: "gate_failed", 14: "unhealthy"}

func (a Agent) evidence(ctx context.Context, w agentapi.Workload, applyErr error) agentapi.Failure {
	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), evidenceTimeout)
	defer cancel()
	candidate := a.candidate(inspectCtx, w.Name)
	failure := agentapi.Failure{
		Class: Classify(applyErr, ctx.Err(), candidate), Container: candidate.Name, ExitCode: candidate.ExitCode,
		OOMKilled: candidate.OOMKilled, HealthLog: candidate.HealthLog, Logs: []string{},
	}
	if candidate.Found {
		failure.Logs = a.logs(inspectCtx, candidate.Name)
	}
	return failure
}

func (a Agent) candidate(ctx context.Context, name string) agentapi.Container {
	newest := agentapi.Container{}
	for _, colour := range []string{"blue", "green"} {
		c, err := a.runtime.Inspect(ctx, name+"-"+colour)
		if err != nil {
			log.Printf("inspect %s-%s: %v", name, colour, err)
		}
		if c.Found && (!newest.Found || c.CreatedAt.After(newest.CreatedAt)) {
			newest = c
		}
	}
	return newest
}

func (a Agent) logs(ctx context.Context, container string) []string {
	lines, err := a.runtime.Logs(ctx, container, failureLogLines)
	if err != nil {
		log.Printf("logs %s: %v", container, err)
		return []string{}
	}
	return lines
}

func Classify(applyErr, ctxErr error, c agentapi.Container) string {
	switch {
	case ctxErr != nil:
		return "timeout"
	case c.OOMKilled:
		return "oom_killed"
	}
	code := exitCode(applyErr)
	if code == 10 {
		return startClass(c)
	}
	if class, ok := stepClasses[code]; ok {
		return class
	}
	return "platform_error"
}

func startClass(c agentapi.Container) string {
	switch {
	case c.Health == "unhealthy":
		return "unhealthy"
	case c.Found && c.State != "running":
		return "crashed"
	}
	return "start_failed"
}

func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 0
}

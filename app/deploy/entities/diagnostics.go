package entities

import "time"

const (
	FailureTimeout   = "timeout"
	FailureOOM       = "oom_killed"
	FailureUnhealthy = "unhealthy"
	FailureCrashed   = "crashed"
	FailureStart     = "start_failed"
	FailureSmoke     = "smoke_failed"
	FailureGate      = "gate_failed"
	FailurePlatform  = "platform_error"

	StaleAfter     = 60 * time.Second
	maxFailureLogs = 50
	maxLogBytes    = 2000
	maxHealthLog   = 3
	maxHealthBytes = 500
)

var FailureClasses = []string{FailureTimeout, FailureOOM, FailureUnhealthy, FailureCrashed, FailureStart, FailureSmoke, FailureGate, FailurePlatform}

var hints = map[string]string{
	FailureUnhealthy: "the container never answered 2xx on health_path: read failure.logs and the deployment log, and check port and health_path in the config spec",
	FailureCrashed:   "the process exited with exit_code: read failure.logs",
	FailureOOM:       "the container ran out of memory: raise memory_mb in the config spec or lower the usage",
	FailureStart:     "the container did not start: read the deployment log for the compose error (image pull, ports, command)",
	FailureSmoke:     "the candidate answered the smoke request with an error: read failure.logs",
	FailureGate:      "errors or latency rose while traffic shifted, so it was rolled back: read failure.logs and listLogs",
	FailureTimeout:   "the deploy took longer than the deploy timeout",
	FailurePlatform:  "the platform failed to deploy: read the deployment log; retrying may help",
}

type Failure struct {
	Class     string   `json:"class"`
	Container string   `json:"container"`
	ExitCode  int      `json:"exit_code"`
	OOMKilled bool     `json:"oom_killed"`
	HealthLog []string `json:"health_log"`
	Logs      []string `json:"logs"`
}

func (f Failure) Hint() string {
	if hint, ok := hints[f.Class]; ok {
		return hint
	}
	return hints[FailurePlatform]
}

func (f Failure) Clipped() Failure {
	f.Logs = clip(f.Logs, maxFailureLogs, maxLogBytes)
	f.HealthLog = clip(f.HealthLog, maxHealthLog, maxHealthBytes)
	return f
}

func clip(lines []string, count, size int) []string {
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line[:min(len(line), size)])
	}
	return out
}

type RuntimeStatus struct {
	ServiceID      string
	Environment    string
	Container      string
	Found          bool
	State          string
	Health         string
	HealthLog      []string
	ExitCode       int
	OOMKilled      bool
	Restarts       int
	Image          string
	MemoryLimitMB  int
	CPULimitMillis int
	StartedAt      *time.Time
	ObservedAt     time.Time
	Agent          string
}

func (s RuntimeStatus) Stale(now time.Time) bool {
	return now.Sub(s.ObservedAt) > StaleAfter
}

type Snapshot struct {
	Service     string
	Environment string
	Status      RuntimeStatus
}

type RuntimeView struct {
	Service     string
	Environment string
	Status      RuntimeStatus
	Stale       bool
	Usage       *Usage
}

type Usage struct {
	CPUMillis      int
	CPULimitMillis int
	MemoryMB       int
	MemoryLimitMB  int
}

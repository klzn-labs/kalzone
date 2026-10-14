# Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Agents read a service's runtime status and logs and get classified deploy failures with evidence through `/api/v1` and `/mcp`.

**Architecture:** deployctl (infra-plateful) exits with a code per failing step. The platform agent inspects containers: every 15 s it reports one snapshot per desired workload, and on a failed deploy it sends the candidate's evidence and a class. The control plane stores snapshots (`runtime_statuses`) and failures (`deployments.failure`), serves `getRuntime` (with CPU/memory from Prometheus through Grafana) in the deploy domain and `listLogs` (Loki through Grafana) in the observe domain.

**Tech Stack:** Go 1.26, gin, fx, gorm/MySQL, golang-migrate, Grafana datasource proxy (Prometheus, Loki).

**Spec:** `docs/superpowers/specs/2026-10-14-diagnostics-design.md`

## Global Constraints

- No comments; function bodies ≤ 20 lines; ≤ 2 levels of control-flow nesting.
- Exit codes: start 10, smoke 11, gate 12, other 13.
- Classes: `timeout`, `oom_killed`, `unhealthy`, `crashed`, `start_failed`, `smoke_failed`, `gate_failed`, `platform_error` (first match in that order).
- Status every `15s` (`AGENT_STATUS_EVERY`); stale after 60 s.
- Failure logs: last 50 lines, ≤ 2000 bytes each; health log ≤ 3 entries of ≤ 500 bytes.
- `listLogs`: `since` default 15m, max 24h; `limit` 1–1000 default 100; `colour` blue|green; scope `logs`, action `logs.read` (member of owning team or admin).
- Datasource uids from config, defaults `prometheus` and `loki`.
- Layered commits, no co-author.

## Review Focus

- `contains` with quotes or backslashes → valid LogQL, no injection into the stream selector. Test in Task 3.
- A snapshot for a service or environment the plane does not know → ignored, not a 500. Test in Task 4.
- Prometheus down or Grafana unset → `getRuntime` still 200 without `usage`. Test in Task 3.
- A failed deploy with no candidate container (image pull) → `start_failed`, `container` empty, no crash. Test in Task 5.
- A token with `logs` but no `read` → refused by the bearer guard's read check (documented: `listLogs` needs `read` and `logs`). Test in Task 4.

---

### Task 1: deployctl exits with a code per step (infra-plateful)

**Files:** `deployctl/internal/deploy/deploy.go`, `deployctl/main.go`, tests in `internal/deploy/deploy_test.go`, `cli_test.go`.

**Produces:** `deploy.Step` (`StepStart`, `StepSmoke`, `StepGate`, `StepOther`), `deploy.StepError{Step, Err}` (`Error`, `Unwrap`), `exitCode(err) int` in main (10/11/12/13, 1 for usage errors stays 13).

- [ ] Tests: start failure → `StepStart`; smoke → `StepSmoke`; gate → `StepGate`; state load → `StepOther`; rollback keeps the cause's step; `exitCode` maps each.
- [ ] Implement: `prepare` wraps `Runtime.Start` (start) and `Smoke.Check` (smoke), route errors other; `shift` wraps `Gate.Watch` (gate); `Run` wraps load-state other; `rollBack` returns an error whose `errors.As` still finds the cause's `StepError`.
- [ ] `go test ./...` in `deployctl`; commit; PR on infra-plateful; merge after CI.

### Task 2: Storage

**Files:** `migrations/000007_diagnostics.{up,down}.sql`; `app/deploy/entities/runtime.go` (+test); `app/deploy/entities/entities.go` (`Failure`, `Deployment.Failure *Failure`); `app/deploy/repositories/mysql/{model.go,deployments.go,runtime.go,runtime_test.go,deployments_test.go}`.

**Produces:**

```go
type Failure struct { Class, Container string; ExitCode int; OOMKilled bool; HealthLog, Logs []string }
func (f Failure) Hint() string
type RuntimeStatus struct {
	ServiceID, Environment, Container, State, Health, Image, Agent string
	Found, OOMKilled bool; HealthLog []string; ExitCode, Restarts, MemoryLimitMB, CPULimitMillis int
	StartedAt *time.Time; ObservedAt time.Time
}
func (s RuntimeStatus) Stale(now time.Time) bool
// repository
func (r DeploymentRepository) SaveRuntime(ctx, []entities.RuntimeStatus) error   // upsert by (service_id, environment)
func (r DeploymentRepository) Runtime(ctx, serviceID, environment string) (entities.RuntimeStatus, bool, error)
```

`Transition` also writes `failure`. Migration: `runtime_statuses` (PK service_id+environment) and `ALTER TABLE deployments ADD COLUMN failure JSON NULL`.

- [ ] Tests (testdb): upsert twice keeps one row with the newer values; `Runtime` unknown → false; a failed transition with a failure reads back equal; `Hint` per class; `Stale` at 60 s.
- [ ] Implement; `go test ./app/deploy/...`; commit.

### Task 3: Usecases and adapters

**Files:** `app/deploy/usecases/agents/{report.go,status.go}` (+tests); `app/deploy/usecases/runtime/runtime.go` (+test); `app/deploy/repositories/metrics/metrics.go` (+test); `app/observe/repositories/grafana/query.go` (+test); `app/observe/usecases/logs/logs.go` (+test); `app/observe/repositories/catalog/catalog.go`; `app/authz/{authz.go,token.go}` (`ReadLogs`); `pkg/agentapi/agentapi.go` (types); `pkg/config/config.go` (uids).

**Produces:**

```go
// agentapi
type Container struct { Found bool; Name, Image, State, Health string; HealthLog []string; ExitCode int; OOMKilled bool; Restarts int; StartedAt, CreatedAt time.Time; MemoryLimitMB, CPULimitMillis int }
type Snapshot struct { Service, Environment string; Container Container }
type StatusReport struct { Workloads []Snapshot }
type Failure struct { Class, Container string; ExitCode int; OOMKilled bool; HealthLog, Logs []string }
// Finish gains Failure *Failure `json:"failure,omitempty"`; StatusPath = "/agent/v1/status"
func (u AgentsUsecase) Finish(ctx, agent, deploymentID string, succeeded bool, message string, failure *entities.Failure) (entities.Deployment, error)
func (u AgentsUsecase) ReportStatus(ctx, agent string, snapshots []entities.Snapshot) error
type RuntimeUsecase; func (u RuntimeUsecase) StatusFor(ctx, a actor.Actor, service, env string) (entities.RuntimeView, error)
type RuntimeView struct { Service, Environment string; Status entities.RuntimeStatus; Stale bool; Usage *entities.Usage }
type Usage struct { CPUMillis, CPULimitMillis, MemoryMB, MemoryLimitMB int }
// grafana
func (c Client) Instant(ctx, uid, query string) (float64, bool, error)
func (c Client) LogLines(ctx, uid, query string, start, end time.Time, limit int) ([]entities.LogLine, error)
type LogsUsecase; func (u LogsUsecase) Read(ctx, a actor.Actor, service, env string, q entities.LogQuery) ([]entities.LogLine, error)
```

- [ ] Tests: Finish stores a clipped failure (50 lines × 2000 bytes, 3 × 500); ReportStatus skips unknown services; StatusFor: not visible → 404, none → 404 with hint, stale, usage merged, usage absent when the querier fails; metrics adapter builds both PromQL strings; LogQL selector + `|=` quoting; query validation (`since`, `limit`, `colour`); logs permission (member yes, outsider 403, token without `logs` 403); Loki JSON parsed and sorted oldest first.
- [ ] Implement; `go test ./app/... ./pkg/...`; commit.

### Task 4: Handlers, wiring and integration tests

**Files:** `app/deploy/handlers/agentapi/agentapi.go`; `app/deploy/handlers/runtime/runtime.go`; `app/deploy/handlers/deployments/dto.go` (`failure`); `app/observe/handlers/logs/logs.go`; `cmd/controlplane/modules/{deploy.go,observe.go}`; `internal/fakegrafana` (Prometheus and Loki proxy answers); `internal/integration/tests/diagnostics_test.go`.

- [ ] Integration tests: agent posts status → `getRuntime` with usage from fakegrafana; unknown service in the report ignored; stale after the clock moves; failed finish with evidence → `getDeployment.failure.class` + hint; `listLogs` lines oldest first, `contains` forwarded; token with `read`+`logs` ok, without `logs` 403, outsider 403, `logs` without `read` 403; MCP lists `getRuntime`/`listLogs` by scope; contract validates responses.
- [ ] Implement; `make test`; commit.

### Task 5: Agent

**Files:** `internal/agent/{agent.go,runtime.go,status.go,failure.go,config.go,client.go}` (+tests); `internal/agent/deployctl/{deployctl.go,inspect.go}` (+tests).

**Produces:** `Runtime` gains `Inspect(ctx, container string) (agentapi.Container, error)` and `Logs(ctx, container string, lines int) ([]string, error)`; `Agent.StatusOnce(ctx) int`; `classify(err error, ctxErr error, c agentapi.Container) string`; `ControlPlane.Status(ctx, []agentapi.Snapshot) error`; `Finish(ctx, id, succeeded, message, *agentapi.Failure)`.

- [ ] Tests: inspect JSON parsed (health log, limits from `HostConfig.Memory`/`NanoCpus`); missing container → `Found: false`; candidate = created last of blue/green; classification table incl. no container; status loop batches all desired; failed run sends evidence; `Observe` still detects drift.
- [ ] Implement; `go test ./internal/agent/...`; commit.

### Task 6: Docs and delivery

- [ ] README "Diagnostics" section (getRuntime, listLogs, failure classes, `logs` scope).
- [ ] Layered history; PR; merge; release (control plane); set `platform_agent_image` and apply Terraform; done-when of spec §1.

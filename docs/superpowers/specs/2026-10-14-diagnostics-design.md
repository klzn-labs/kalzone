# Diagnostics — design

Date: 2026-10-14
Status: approved in conversation (the user told the agent on 2026-10-14 to follow the recommended option at every decision; the research decisions of 2026-10-12 bind), open for review
Sub-project 5 of 7 of "a platform made for AI agents". Builds on sub-projects 1–4, all live.

## 1. Why

An agent can build, deploy and wait through `/api/v1` and `/mcp`, but when a deploy
fails or a service misbehaves it only gets `status: failed` and a message such as
`deployctl: exit status 1: … | …`. Finding out whether the container crashed, ran out
of memory, failed its health check or was rolled back by the traffic gate needs SSH to
the VPS, `docker inspect`, `docker logs` and Grafana. On 2026-10-12 every such
question went through a human.

**Done when**, on production, through `/mcp` with the `claude-local` token:

- `getRuntime` for `hello` in `dev` shows the live container running and healthy,
  its restarts, and its CPU and memory next to the limits;
- `listLogs` for `hello` in `dev` returns the service's recent lines;
- a deploy of `hello` to `dev` that cannot pass its health check (a `health_path`
  that answers 404) ends `failed` with `failure.class: unhealthy`, the candidate's last
  log lines, its health check output and a hint; restoring the config and deploying
  again succeeds.

## 2. Decisions

| # | Topic | Decision |
|---|---|---|
| 1 | Runtime status source | The agent `docker inspect`s the live container of every desired workload every **15 s** and reports one snapshot per workload. CPU and memory come from Prometheus (cadvisor) when the status is read. |
| 2 | Logs source | Loki (already fed by Alloy with a `container` label), read by the control plane through Grafana's datasource proxy with the existing Grafana token. |
| 3 | Kept on failure | The agent collects the failed candidate's **last 50 log lines** and its inspect evidence before reporting the failure; they are stored on the deployment and do not depend on Loki. |
| 4 | Classification | deployctl exits with a **code per failing step**; the agent turns that code and the candidate's inspect evidence into one class. The control plane adds the hint for the class. |
| 5 | Logs permission | A new `logs.read` action: a member of the service's owning team or a platform admin; a token also needs the `logs` scope (already in the scope list since sub-project 1). Runtime status is a read. |
| 6 | UI | Out of scope: API and MCP only. The web UI can show the same data later. |

## 3. deployctl exit codes (infra-plateful)

`deployctl deploy` today exits `1` for every failure. It exits instead with:

| Code | Step that failed |
|---|---|
| 10 | start: `docker compose up --wait` (image pull, container exit, health check) |
| 11 | smoke check of the candidate |
| 12 | traffic gate (error rate or latency verdict while shifting or baking) |
| 13 | anything else: state, routes, recorder |

Each step wraps its error in a `deploy.StepError{Step, Err}`; `main` maps the step to
the code with `errors.As`. A rollback keeps the code of the step that caused it. The
platform's release workflow already installs deployctl from infra-plateful's `main`
before deploying the control plane.

## 4. Agent (platform `internal/agent`)

**Inspect.** The deployctl runtime gets `Inspect(ctx, container) (Container, error)`:
one `docker inspect` returning `name`, `image`, `state` (`running`, `exited`,
`restarting`, …), `health` (`healthy`, `unhealthy`, `starting`, or empty without a
health check), the last 3 health check outputs, `exit_code`, `oom_killed`,
`restarts`, `started_at` and `created_at`. A missing container is `Found: false`, not
an error. `Observe` is rebuilt on `Inspect`.

**Status loop.** A third ticker, `AGENT_STATUS_EVERY` (default `15s`): read the desired
workloads, inspect each live container (`<name>-<colour>` from the state file), and send
all snapshots in one `POST /agent/v1/status` `{"workloads": [...]}`. A workload
never deployed on this host is sent with `found: false`.

**Failure evidence.** When `Apply` fails, the agent:

1. finds the candidate: of `<name>-blue` and `<name>-green`, the one created last;
2. inspects it and reads `docker logs --tail 50 --timestamps` (stdout and stderr);
3. classifies (below) and sends `Finish{succeeded: false, message, failure}`.

**Classes**, first match wins:

| Class | When |
|---|---|
| `timeout` | the deploy's context ended (deploy timeout) |
| `oom_killed` | the candidate's `oom_killed` |
| `unhealthy` | exit 10 and health `unhealthy` |
| `crashed` | exit 10 and the candidate is not running (exited or restarting) |
| `start_failed` | exit 10 otherwise (image pull, compose error, no container) |
| `smoke_failed` | exit 11 |
| `gate_failed` | exit 12 |
| `platform_error` | exit 13, or any other error |

`failure` = `{class, container, exit_code, oom_killed, health_log, logs}`, with
`container` empty when there was none.

## 5. Control plane

**Storage** (migration `000007_diagnostics`):

- `runtime_statuses`: one row per `(service_id, environment)`, upserted:
  `container`, `found`, `state`, `health`, `health_log` (JSON), `exit_code`,
  `oom_killed`, `restarts`, `image`, `started_at`, `observed_at`, `agent`.
- `deployments.failure`: JSON, null unless failed with evidence.

**Agent API.** `POST /agent/v1/status` (agent token) upserts every snapshot of a
known `(service, environment)` and ignores unknown ones. `POST /agent/v1/deployments/:id/finish`
accepts the optional `failure`.

**`GET /api/v1/runtime/:service/:env`** — `getRuntime`, scope `read`:

```json
{
  "service": "hello", "environment": "dev",
  "container": "hello-dev-green", "found": true,
  "state": "running", "health": "healthy", "health_log": ["…"],
  "restarts": 0, "exit_code": 0, "oom_killed": false,
  "image": "docker.io/adrianozdp/hello-platform@sha256:…",
  "started_at": "…", "observed_at": "…", "stale": false,
  "usage": {"cpu_millis": 3, "cpu_limit_millis": 250, "memory_mb": 12, "memory_limit_mb": 128}
}
```

- `stale` is true when `observed_at` is older than 60 s (the agent stopped reporting).
- `usage` comes from two instant Prometheus queries through
  `GET <grafana>/api/datasources/proxy/uid/prometheus/api/v1/query`:
  `sum(rate(container_cpu_usage_seconds_total{name="<container>"}[1m])) * 1000` and
  `sum(container_memory_working_set_bytes{name="<container>"}) / 1048576`, rounded;
  limits from the live config's spec. When Grafana is not configured or the query fails,
  `usage` is left out and the answer is still `200`.
- No snapshot yet → `404` with hint "the agent reports every 15s; deploy the service
  first".

**`GET /api/v1/logs/:service/:env`** — `listLogs`, scope `logs`, action `logs.read`:

| Query | Meaning |
|---|---|
| `since` | duration, default `15m`, at most `24h` |
| `limit` | 1–1000, default 100: the newest N lines in the window |
| `contains` | optional substring (Loki line filter `|=`) |
| `colour` | optional `blue` or `green`; both by default |

Answer `{"items": [{"at", "container", "line"}]}`, oldest first, no `next_cursor`.
Loki query `{container=~"<name>-(blue|green)"}` through
`/api/datasources/proxy/uid/loki/loki/api/v1/query_range` with `direction=backward`.
`contains` is quoted for LogQL (backslashes and quotes escaped). Grafana not configured
→ `503 err_not_configured`; Loki failing → `502 err_upstream`.

**Deployment DTO** gains `failure` when present:
`{class, hint, container, exit_code, oom_killed, health_log, logs}`. Hints by class:

| Class | Hint |
|---|---|
| `unhealthy` | the health check at `health_path` kept failing: read `failure.health_log` and `failure.logs`, and check that the path answers 2xx inside the container |
| `crashed` | the process exited with `exit_code`: read `failure.logs` |
| `oom_killed` | the container ran out of memory: raise `memory_mb` in the config spec or lower the usage |
| `start_failed` | the container did not start: read the deployment log for the compose error (image pull, ports, command) |
| `smoke_failed` | the candidate answered the smoke request with an error: read `failure.logs` |
| `gate_failed` | errors or latency rose while traffic shifted, so it was rolled back: read `failure.logs` and listLogs |
| `timeout` | the deploy took longer than the deploy timeout |
| `platform_error` | the platform failed to deploy: read the deployment log; retrying may help |

The datasource uids are config: `PLATFORM_GRAFANA_PROMETHEUS_UID` (`prometheus`) and
`PLATFORM_GRAFANA_LOKI_UID` (`loki`).

**MCP.** `getRuntime` and `listLogs` become tools automatically (sub-project 4);
`listLogs` shows only for tokens with `logs`.

## 6. Errors and limits

- A token without `logs` gets `403 err_missing_scope` on `listLogs`; a human outside the
  owning team gets `403`.
- `since` over 24h or not a duration, `limit` out of range, `colour` unknown → `400` with
  `field` and hint.
- Snapshots from an agent are trusted like its other reports (agent token).
- Failure logs are capped at 50 lines of at most 2000 bytes each; health log at 3
  entries of 500 bytes.

## 7. Testing

- deployctl: each step's failure exits with its code (`cli_test`).
- agent: inspect parsing from recorded `docker inspect` JSON (fake docker script, as the
  existing runtime tests do); candidate choice; classification table; status loop sends
  one batch; failure evidence sent with Finish.
- control plane unit: usecases for snapshot upsert, staleness, usage merge, logs
  query building and LogQL quoting, permission.
- integration: agent posts status → `getRuntime` (fakegrafana answers the Prometheus
  proxy); `listLogs` against fakegrafana's Loki proxy, with `logs` scope and without;
  a failed finish with evidence → `getDeployment.failure` with the hint; both new
  operations appear as MCP tools for the right tokens; contract tests validate the new
  responses.

## 8. Delivery

- infra-plateful PR: deployctl exit codes; merged before the platform release, which
  installs it.
- platform PR, layers: migration + repositories; usecases (runtime, logs, failure);
  handlers + agent API + wiring + integration tests; agent (inspect, status loop,
  evidence); docs. Release blue-green (control plane). The release only pushes the agent
  image: the agent is rolled out by setting `platform_agent_image` in infra-plateful's
  tfvars to the release's image and applying Terraform (Docker, whole repo mounted).
- infra: add the two uid variables only if their defaults do not match (they match the
  provisioned datasources: `prometheus`, `loki`).
- Done-when of §1 from this machine.

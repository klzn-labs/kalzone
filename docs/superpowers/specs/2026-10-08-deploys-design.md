# Platform deploys and the agent — design

Date: 2026-10-08
Status: self-approved (user asleep, asked Claude to decide), open for review in the PR
Sub-project 3 of 5. Builds on core and releases.

## 1. Goal

Run releases. A **deployment** puts one built release with one config revision into
one environment. The control plane stores what should run; an **agent** on the host
claims the work, performs the rollout, reports progress, and keeps checking that
what runs is still what should run.

Agreed with the user on 2026-10-07: control plane holds desired state, an agent
reconciles, the agent holds the Docker socket and the control plane never does.

## 2. Deployment

| Field | Notes |
|---|---|
| id, service_id, environment, number | number counts per service and environment |
| release_id, release_number, image_ref | `image@sha256:…`, copied from the built release |
| config_revision | the revision number deployed (0 = defaults) |
| strategy | `bluegreen` (default) or `canary` |
| trigger | `manual`, `promote`, `rollback`, `config`, `reconcile` |
| status | `pending` → `in_progress` → `succeeded` \| `failed`; `pending` → `cancelled` |
| requested_by, agent, message | who asked, which agent ran it, last outcome |
| created_at, started_at, finished_at | |

Progress lines go to `deployment_events` (deployment_id, at, message).

Rules:

- Only `built` releases of the same service can be deployed.
- At most one deployment per service and environment is `in_progress`. Asking for
  a new one while one runs is a conflict; asking while one is `pending` cancels the
  pending one, so only the latest intent waits.
- **Desired state** of a service in an environment is its latest `succeeded`
  deployment.
- **Deploy**: a chosen release with the environment's *current* config revision.
- **Apply config**: the live release with the current config revision.
- **Promote**: the release live in the source environment, with the *target's*
  current config revision. dev → prod is the only path the UI offers.
- **Rollback**: an earlier succeeded deployment's release **and** config revision,
  exactly as it was; this is what the immutable revisions of the core are for.
- Permission: `deploy` (member on dev, maintainer on protected environments), the
  same row of the matrix the core already defined.

## 3. Agent protocol

Agents authenticate with `Authorization: Bearer <PLATFORM_AGENT_TOKEN>` and name
themselves with `X-Agent-Name`. Every call is a heartbeat.

| Call | Purpose |
|---|---|
| `POST /agent/v1/claim` | oldest `pending` deployment becomes `in_progress` for this agent; returns the workload or 204 |
| `POST /agent/v1/deployments/{id}/events` | append progress lines |
| `POST /agent/v1/deployments/{id}/finish` | `{succeeded, message}` |
| `GET /agent/v1/desired` | the desired workload of every service and environment |
| `POST /agent/v1/drift` | `{service, environment, observed}`: what runs differs from desired |

A workload carries everything needed to run it: name (`<service>-<env>`), image
reference, the runtime spec, plain variables and **decrypted** secrets. Secrets
leave the control plane only here, over TLS, to an authenticated agent.

Claiming is a compare-and-set (`UPDATE … WHERE status = 'pending'`), so two agents
cannot run the same deployment. A deployment `in_progress` for over 30 minutes is
failed by the control plane's sweeper ("agent stopped reporting").

Drift creates a deployment with trigger `reconcile` that re-applies the desired
release and config, unless one is already pending or running. Every correction is
therefore visible in the history and the audit log.

## 4. The agent

`cmd/agent`, a separate binary and image (`adrianozdp/platform-agent`), runs on the
VPS with the Docker socket and `/opt/plateful`. Its loop:

1. every 5 s: claim; if there is work, run it and stream each output line as an event;
2. every 60 s: fetch desired state, observe each workload, report drift.

It reaches the runtime through one port:

```go
type Runtime interface {
    Apply(ctx context.Context, w Workload, out io.Writer) error
    Observe(ctx context.Context, name string) (Observed, error)
}
```

### The deployctl runtime

The compose host already has a tested rollout engine: deployctl (blue-green and
canary, Traefik weighted routing, smoke test, Prometheus gate, automatic rollback,
Grafana annotations). The agent's first runtime drives it:

- writes `stack/deploy/<name>.yml` from the spec (port, health path, memory, a
  `Host()` rule on `websecure` with TLS when the spec has a host, otherwise a
  worker with the spec's command),
- writes `config/<name>.env` (plain variables) and `secrets/<name>.env` (mode 600),
- runs `deployctl deploy -root /opt/plateful -service <name> -image <image@digest>
  -version r<release>.c<config> -strategy <strategy>`,
- observes through deployctl's `state/<name>.json` and `docker inspect` of the live
  colour.

**Why the CLI and not the library**: deployctl's engine is in `internal/` packages of
the private infra repository, whose module path (`adrianozdp/…`) does not match the
repository (`adrianozp/…`), so it cannot be imported as is. Moving it needs a
change in infra first. The `Runtime` port keeps that swap, and a later Kubernetes
runtime, local to one adapter.

Limits of this runtime: one container per colour (replicas above 1 are ignored),
memory is limited but CPU is not (compose.app.yml has no CPU limit).

## 5. Surface

- Environment tab: a **Live** card (release, config revision, status, when), a
  **Deploy** form (built release, strategy), **Apply config r<n>** when the live
  config is behind, **Promote from dev** on prod, deployment history with
  **Roll back** on earlier successes, and a live log while a deployment runs.
- Overview chips and service cards show the live release per environment.
- Admin page **Agents**: name, version, last seen.
- API: `GET /api/v1/deployments?service=&env=`, `POST /api/v1/deployments`
  `{service, environment, release_id, strategy}`, `POST /api/v1/deployments/promotions`
  `{service, from, to}`, `POST /api/v1/deployments/{id}/rollback`,
  `POST /api/v1/deployments/config-applications` `{service, environment}`,
  `GET /api/v1/deployments/{id}` (with events).

## 6. Config spec addition

`command` (optional) joins the runtime spec. A service without a public host runs
as a worker and needs a command.

## 7. Configuration

Control plane: `PLATFORM_AGENT_TOKEN` (agents are refused without it),
`PLATFORM_DEPLOY_TIMEOUT` (default `30m`).

Agent: `AGENT_PLATFORM_URL`, `AGENT_TOKEN`, `AGENT_NAME` (default hostname),
`AGENT_ROOT` (default `/opt/plateful`), `AGENT_DEPLOYCTL`
(default `/opt/plateful/bin/deployctl`), `AGENT_POLL` (5 s), `AGENT_DRIFT_EVERY` (60 s).

## 8. Testing

- Usecases: deploy rules (built only, same service, one in progress, pending
  superseded), promote picks the source's live release and the target's config,
  rollback reuses the old config revision, claim is exclusive, drift creates one
  reconcile deployment, sweeper.
- deployctl runtime against a fake `deployctl` script and a temp root: files written
  with the right content and modes, arguments, exit codes, observation.
- Agent loop against a fake control plane.
- Integration: deploy → agent claims over HTTP → events → finish → live state in API
  and UI; secrets reach only the agent endpoint.

# Platform

An internal developer platform for the Plateful services, built as platform-engineering
practice. Teams register their services, keep per-environment config and secrets, and
(in later sub-projects) build, release, deploy and watch them from one website.

This repository holds the **control plane**: a Go service with an htmx web UI and a JSON
API on the same usecases. Design documents live in `docs/superpowers/specs`.

## What is in the core

- GitHub sign-in (GitHub App user OAuth), server-side sessions, CSRF protection
- Access tokens for agents and scripts: Bearer tokens owned by a person, limited by scopes and teams
- Approvals: an agent's change to a protected environment waits as a request until a human approves it
- Teams with `member` and `maintainer` roles, platform admins
- Service catalog, fixed `dev` and `prod` environments (`prod` is protected)
- Config per service and environment as immutable, numbered revisions
- Secrets encrypted with AES-256-GCM, write-only, versioned with the config
- One authorization policy (`app/authz`) used by every usecase
- Audit log of every change
- Deploys: a deployment runs one built release with one config revision in one
  environment; promote dev to prod, apply new config, roll back to any earlier
  success. An agent on the host claims deployments, rolls them out with deployctl
  and reports drift, which the platform turns into a reconcile deployment
- Observability: Grafana behind the platform's sign-in at `/grafana`, a dashboard
  per service written through Grafana's API, and its panels embedded in each
  environment tab (metrics from Prometheus, logs from Loki)
- Releases: immutable, digest-pinned images. The platform triggers builds in GitHub
  Actions through a GitHub App, builds report back with a GitHub OIDC token, and
  third-party images are registered by tag

## Layout

```
cmd/controlplane              entrypoint and fx composition root
app/<domain>/entities         identity, team, catalog, envconfig, audit
app/<domain>/usecases         business rules
app/<domain>/repositories     MySQL adapters and adapters to other domains
app/<domain>/handlers         JSON API under /api/v1
app/authz                     the permission matrix
app/web                       htmx pages, templates and static assets
internal                      HTTP server, sessions, secret box, event bus, tests
migrations                    schema, applied on start
```

## Running locally

Needs Go 1.26 and a MySQL 8 server.

```
docker run -d --name platform-mysql -e MYSQL_ROOT_PASSWORD=test -p 3306:3306 mysql:8
docker exec platform-mysql mysql -uroot -ptest -e "CREATE DATABASE platform"
PLATFORM_GITHUB_CLIENT_ID=... PLATFORM_GITHUB_CLIENT_SECRET=... PLATFORM_ADMINS=your-login make run
```

Open http://localhost:8080. The GitHub App needs `http://localhost:8080/auth/github/callback`
as a callback URL.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PLATFORM_HTTP_ADDR` | `:8080` | listen address |
| `PLATFORM_BASE_URL` | `http://localhost:8080` | public URL, used for the OAuth callback and secure cookies |
| `PLATFORM_DB_DSN` | `root:test@tcp(127.0.0.1:3306)/platform` | MySQL DSN |
| `PLATFORM_GITHUB_CLIENT_ID` / `_SECRET` | | GitHub App user-auth credentials |
| `PLATFORM_ADMINS` | | comma-separated GitHub logins that are always admin |
| `PLATFORM_SECRETS_KEY` | a development key | base64 of 32 random bytes: `openssl rand -base64 32` |
| `PLATFORM_SECRETS_KEY_ID` | `k1` | stored next to each ciphertext for key rotation |
| `PLATFORM_ALLOWED_DOMAINS` | `localhost` | hosts a service may claim must end with one of these |
| `PLATFORM_SESSION_TTL` | `168h` | session lifetime, renewed when less than a day is left |
| `PLATFORM_GITHUB_APP_ID` | | GitHub App id, needed to start builds |
| `PLATFORM_GITHUB_APP_PRIVATE_KEY` | | the App's PEM private key (`\n` escapes allowed) |
| `PLATFORM_GITHUB_WEBHOOK_SECRET` | | secret GitHub signs webhooks with |
| `PLATFORM_OIDC_ISSUER` | `https://token.actions.githubusercontent.com` | issuer of build tokens |
| `PLATFORM_OIDC_AUDIENCE` | the base URL | audience build tokens must carry |
| `PLATFORM_BUILD_TIMEOUT` | `60m` | builds still running after this are marked failed |
| `PLATFORM_AGENT_TOKEN` | | shared secret agents authenticate with; agents are refused without it |
| `PLATFORM_GRAFANA_URL` | | Grafana inside the network, e.g. `http://grafana:3000/grafana`; proxied at `/grafana` with single sign-on |
| `PLATFORM_GRAFANA_TOKEN` | | Grafana service-account token (Editor) the platform writes dashboards with |
| `PLATFORM_DEPLOY_TIMEOUT` | `30m` | deployments an agent stopped reporting on are failed after this |
| `PLATFORM_QUEUES_TOKEN` | | shared secret the queue gateway authenticates with; the gateway API answers 503 without it |
| `PLATFORM_QUEUES_GATEWAY_URL` | `http://queued:8080` | gateway address handed to services as `PLATFORM_QUEUE_URL` |
| `PLATFORM_APPROVALS_WEBHOOK_URL` | | Discord webhook told about every new change request; empty sends nothing |

Never run production with the default secrets key.

## Access tokens

Agents call the API with a token instead of a browser session. Create one under
**Access tokens** (key icon next to your name) or with the API:

```
curl -H "Authorization: Bearer $PLATFORM_TOKEN" https://platform.kalz.one/api/v1/whoami
```

A token acts for the person who made it and can do at most what that person, its
scopes and its teams all allow. Scopes: `read`, `logs` (log endpoints come later;
`read` does not include it), `services`, `services:admin`, `builds`, `config`,
`secrets`, `deploy`, `protected`, `teams:admin`, `tokens:admin`. Teams are listed
explicitly. Revoking a token frees its name; a token made by another token never
outlives it. Tokens expire after 1 to 365 days (90 by default) and are shown
once; only a hash is stored.

On a protected environment (`prod`) a token needs `protected`, and then its change
waits for approval (see Approvals) unless a platform admin gave it bypass from the website.
Tokens are refused on website pages and need no CSRF header. Every change made
with a token shows in the audit log as "person via token".

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/whoami` | owner, and for a token its scopes, team slugs, bypass and expiry (a browser session gets the owner only) |
| `GET /api/v1/tokens` | your tokens with their team slugs (`?all=true` for admins in the browser) |
| `POST /api/v1/tokens` | `{"name","scopes","teams","expires_in_days","bypass"}` → the token, once |
| `DELETE /api/v1/tokens/:id` | revoke |

## API for agents

`GET /api/v1/openapi.json` describes every operation: its `operationId`, the token
scope it needs, path and query parameters, the request body, every response, and the
values of every status field. Any valid token or session reads it. It is generated
from the handlers' declarations, so it is always the API that runs.

- A single resource is the bare object; actions that start work (build, deploy,
  promote, config apply, rollback) answer `202` with it.
- A list is `{"items": [...], "next_cursor": "..."}`: `?limit` 1-100 (20 by
  default), and `?cursor=<next_cursor>` for the next page; no `next_cursor` means
  the end.
- An error is `{"error": {"code", "message", "hint", "field", "details"}}`; `hint`
  says how to fix the call and `field` names the offending field or parameter.
  `hint`, `field` and `details` appear only when they apply.
- Bodies are strict: an unknown field is a `400` that lists the accepted fields.
- `GET /api/v1/deployments/:id` carries `events_summary` (`total` lines,
  `progress_lines` counting the docker layer progress lines the tail skips, `phases` with the seconds spent waiting and running, the
  last 20 other lines as `tail`, and the `log` path); the full log is
  `GET /api/v1/deployments/:id/events`, oldest first. `?tail=N` (1-100) answers the
  last N lines instead and cannot be combined with `cursor`. While a deployment
  runs, `next_cursor` stays so the log can be followed.
- `?wait=30s` (above 0, at most `120s`) on `GET` of a release, deployment or change
  request, and on the calls that start a build, deploy, promotion, config apply or
  rollback, answers when the status is final with `200`, or when the wait ends with
  `202`, the current state and `Retry-After: 5`. A call that became a change request
  waits for the request instead.
- A release is named by its id or, within a service, by `latest` (the last built),
  `r42`, `sha:<7 to 40 hex>` (the newest build of that commit; a prefix shared by two
  commits is refused) or a tag: in `release` of `POST /api/v1/deployments`, and in
  `GET /api/v1/releases/<ref>?service=<slug>`.
- `POST /api/v1/deployments` and `POST /api/v1/deployments/promotions` take
  `targets` (1 to 10 `{"service", "environment"}`) instead of one service: every
  target is checked first, then all are deployed in one transaction, or none is. The
  answer is `{"items": [...]}` in target order. A batch with a target that needs
  approval is refused; send those targets one at a time.
- `PATCH /api/v1/config/:service/:env` with `Content-Type: application/merge-patch+json`
  writes a revision that changes only what the patch names in `spec` and `env`;
  `null` removes a field or a variable, and `spec: null` or `env: null` is a `400`.
  Secrets keep their own endpoints.
- `GET /api/v1/config/search?key=LOG_LEVEL` lists where a variable or secret is set in
  the current configs of the services the caller sees, narrowed by `env` and `team`;
  the key is matched exactly and secrets are listed by name only.

## Approvals

A token with `protected` but without bypass does not change a protected environment
directly. Config revisions, secrets, deploys, promotions, rollbacks, config
applications and queue create, update and delete answer `202`:

```json
{"kind": "change_request", "id": "…", "status": "pending", "summary": "deploy release #14 to prod",
 "approve_url": "https://platform.kalz.one/requests/…", "poll": "/api/v1/requests/…", "expires_at": "…"}
```

A maintainer of the service's team or an admin, the token's owner included, opens
**Requests** in the browser, sees the change against what runs now, and approves or
rejects it. Approval runs the change as the token, re-applied on the current state; it
fails with `err_stale` for config and deploy changes only when the same config key
changed or another deployment went live in the meantime, and a queue update fails with
`err_conflict` when it finds the queue changed since the request. A newer request for the same thing cancels the older one,
and requests expire after 24 hours. Promote pins the release that was live when asked.
Queue pause, resume and dead-letter commands still answer `err_approval_required`.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/requests/:id` | `status` (`pending`, `approved`, `executed`, `failed`, `rejected`, `cancelled`, `expired`), `result`, `reason`, `error` |
| `GET /api/v1/requests?status=pending` | the token's teams' requests, newest first, paged |
| `POST /api/v1/requests/:id/cancel` | withdraw your own request |

## Builds and releases

1. Give the GitHub App the repository permission **Actions: read and write**,
   subscribe it to **push** and **workflow run** events with the webhook URL
   `https://platform.<domain>/webhooks/github`, and install it on the service's
   repository.
2. Allow the service's repository to use workflows from this repository
   (Settings → Actions → General → Access) because this repository is private.
3. Add `.github/workflows/platform-build.yml` to the service's repository. The
   Releases tab of every service shows the file; it calls
   [`build.yml`](.github/workflows/build.yml) here, which builds and pushes the
   image and reports the digest with the job's OIDC token. The repository needs
   `DOCKER_USERNAME` and `DOCKER_PASSWORD`; it never holds a platform secret.
   The build always passes `VERSION=<commit>`; more Docker build args go in the
   `build_args` input, one `NAME=value` per line, for example
   `build_args: VITE_GOOGLE_CLIENT_ID=${{ vars.GOOGLE_CLIENT_ID }}`.
4. Build a ref from the Releases tab, or switch on auto-build to build every
   push to the default branch.

## Queues

Teams create work queues from the API (`/api/v1/queues`): one consumer service per
queue and environment, a list of publisher services, and settings for rate limit,
in-flight deliveries, attempts, backoff, delivery timeout, retention and message size.
Each change is a numbered version in the audit log. A per-host gateway, `queued`,
reads the queues from `/queued/v1/desired` and runs them; see
`docs/superpowers/specs/2026-10-08-managed-queues-design.md`.

Every deployed service receives `PLATFORM_QUEUE_URL`, `PLATFORM_QUEUE_TOKEN` (to
publish) and `PLATFORM_QUEUE_SIGNING_KEY` (to verify deliveries). The platform sets
them; the config API refuses those names.

## Queue gateway

`cmd/queued` runs the queues, built into `adrianozdp/platform-queued`. It takes
publishes on `POST /v1/queues/{name}/messages` (bearer `PLATFORM_QUEUE_TOKEN`,
optional `Idempotency-Key` and `Platform-Delay` in seconds), keeps messages in NATS
JetStream (streams `Q_`, `QD_` for delayed and `DLQ_` for dead letters per queue),
and delivers each with a signed POST to the consumer under the queue's rate limit,
in-flight limit and retry policy. It polls the control plane for queues and
commands and reports stats; Prometheus metrics are on the metrics address.

| Variable | Default | Purpose |
|---|---|---|
| `QUEUED_PLATFORM_URL` | | control plane base URL |
| `QUEUED_TOKEN` | | the control plane's `PLATFORM_QUEUES_TOKEN` |
| `QUEUED_NATS_URL` / `_USER` / `_PASSWORD` | | JetStream connection |
| `QUEUED_ADDR` | `:8080` | publish endpoint |
| `QUEUED_METRICS_ADDR` | `:9090` | `/metrics` |
| `QUEUED_POLL` | `5s` | desired state poll |
| `QUEUED_STATS_EVERY` | `15s` | stats report |

## The agent

`cmd/agent` runs on each host, built into `adrianozdp/platform-agent`. It needs the
Docker socket, the deployctl root and host networking (deployctl talks to
Prometheus and Traefik on 127.0.0.1):

```
docker run -d --name platform-agent --network host --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock -v /opt/plateful:/opt/plateful \
  -e AGENT_PLATFORM_URL=https://platform.kalz.one -e AGENT_TOKEN=... \
  adrianozdp/platform-agent:<sha>
```

| Variable | Default | Meaning |
|---|---|---|
| `AGENT_PLATFORM_URL` | | control plane URL |
| `AGENT_TOKEN` | | the control plane's `PLATFORM_AGENT_TOKEN` |
| `AGENT_NAME` | host name | how the agent shows on the Agents page |
| `AGENT_ROOT` | `/opt/plateful` | deployctl root |
| `AGENT_DEPLOYCTL` | `/opt/plateful/bin/deployctl` | deployctl binary |
| `AGENT_POLL` | `5s` | how often it asks for work |
| `AGENT_DRIFT_EVERY` | `60s` | how often it compares what runs with what should |

Each service runs as the deployctl service `<service>-<environment>`. A service
with a public host gets a `Host()` rule on Traefik's TLS entrypoint; one without
runs as a worker and needs a command. Replicas and CPU limits are not applied by
this runtime yet.

## Tests

```
make test
```

Integration tests need MySQL; they use `PLATFORM_TEST_DSN`
(default `root:test@tcp(127.0.0.1:3306)/platform_test`) and create the database.
Tests share it, so they run with `-p 1`.

## Releasing

`Release` in GitHub Actions builds `adrianozdp/platform-controlplane` and deploys it
blue-green through infra-plateful's deployctl, like the Plateful apps. The repository
needs the same secrets as go-plateful: `DOCKER_USERNAME`, `DOCKER_PASSWORD`,
`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` and `INFRA_DEPLOY_KEY`.

# Development

This repository is the **control plane**: a Go service with an htmx web UI and a JSON
API on the same usecases.

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
- Observability: Grafana behind the platform's sign-in at `/grafana`, one Grafana org
  per team holding its members, its services' dashboards and datasources that read
  only its own metrics and logs, and those panels embedded in each environment tab
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
make run
```

Open the setup link the control plane logs on its first start and create the owner.

## Sign-in modes

An instance signs people in one way, chosen at start:

- **Password mode**, when `PLATFORM_GITHUB_CLIENT_ID` or `PLATFORM_GITHUB_CLIENT_SECRET`
  is empty. People sign in with a username and password. While nobody has an account,
  every start logs a one-time setup link (24 hours):
  `WARN setup: open <base>/join/<token> to create the owner`. The owner becomes an
  admin, maintainer of the `default` team, and gets an `agent` token with the
  `claude mcp add` command. Admins and team maintainers bring others in with invite
  links (7 days) from the team page or the `createInvite` operation; admins make
  password reset links (24 hours) and remove people on the People page. Each link
  works once.
- **GitHub mode**, when both are set. People sign in with GitHub as before, and every
  password feature is off: no password form, no setup link, `/join/…` answers 404, and
  the invite and reset operations answer `409 err_github_mode`. `PLATFORM_ADMINS`
  applies only here. The GitHub App needs `http://localhost:8080/auth/github/callback`
  as a callback URL:

  ```
  PLATFORM_GITHUB_CLIENT_ID=... PLATFORM_GITHUB_CLIENT_SECRET=... PLATFORM_ADMINS=your-login make run
  ```

Switching an instance from one mode to the other is not supported. Password users keep
their rows but cannot sign in once GitHub is configured, and GitHub users cannot sign in
once it is removed.

On the host, the same binary prints links without starting the server:

```
controlplane setup-link
controlplane setup-link --reset <login>
```

The first prints a new setup link while there is no owner and refuses once there is.
The second prints a password reset link for that person at any time, which is how a
locked-out owner gets back in. Both print the URL on stdout, or the reason on stderr
with exit status 1, and refuse in GitHub mode.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PLATFORM_HTTP_ADDR` | `:8080` | listen address |
| `PLATFORM_BASE_URL` | `http://localhost:8080` | public URL, used for the OAuth callback, setup, invite and reset links, and secure cookies |
| `PLATFORM_DB_DSN` | `root:test@tcp(127.0.0.1:3306)/platform` | MySQL DSN |
| `PLATFORM_GITHUB_CLIENT_ID` / `_SECRET` | | GitHub App user-auth credentials; when both are set the instance is in GitHub mode, otherwise in password mode |
| `PLATFORM_ADMINS` | | comma-separated GitHub logins that are always admin; GitHub mode only |
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
| `PLATFORM_GRAFANA_ADMIN_USER` | | Grafana server admin the platform manages orgs, people, datasources and dashboards as |
| `PLATFORM_GRAFANA_ADMIN_PASSWORD` | | Its password |
| `PLATFORM_TELEMETRY_KEY` | | Signs each team's telemetry token; without it `/telemetry` answers 503 |
| `PLATFORM_TELEMETRY_ALL_TOKEN` | | Bearer token of the `all` scope, which Grafana's main org reads every Loki tenant with |
| `PLATFORM_TELEMETRY_INTERNAL_URL` | `http://controlplane:8080` | Where Grafana reaches the control plane's `/telemetry` |
| `PLATFORM_TELEMETRY_PROMETHEUS_URL` | `http://prometheus:9090` | Prometheus the team telemetry filter reads |
| `PLATFORM_TELEMETRY_LOKI_URL` | `http://loki:3100` | Loki the team telemetry filter reads, with one tenant per service and environment |
| `PLATFORM_DEPLOY_TIMEOUT` | `30m` | deployments an agent stopped reporting on are failed after this |
| `PLATFORM_QUEUES_TOKEN` | | shared secret the queue gateway authenticates with; the gateway API answers 503 without it |
| `PLATFORM_QUEUES_GATEWAY_URL` | `http://queued:8080` | gateway address handed to services as `PLATFORM_QUEUE_URL` |
| `PLATFORM_APPROVALS_WEBHOOK_URL` | | Discord webhook told about every new change request and every deployment that succeeds on prod; empty sends nothing |
| `PLATFORM_HEALTH_RESPONSE_MS` | `300` | prod p95 response time at or above which a service is broken, unless the service sets its own |
| `PLATFORM_HEALTH_CPU_PERCENT` | `70` | busiest prod container's CPU, as a share of its `cpu_millis`, at or above which a service is broken |
| `PLATFORM_HEALTH_DB_CPU_PERCENT` | `70` | CPU of the shared MySQL, in percent of one core, at or above which every service with a prod database is broken |

Never run production with the default secrets key.

## Tests

```
make test
```

Integration tests need MySQL; they use `PLATFORM_TEST_DSN`
(default `root:test@tcp(127.0.0.1:3306)/platform_test`) and create the database.
Tests share it, so they run with `-p 1`.

## Releasing

`Release` in GitHub Actions builds `adrianozdp/platform-controlplane` and deploys it
blue-green through [kalzone-infra](https://github.com/klzn-labs/kalzone-infra)'s deployctl.
The repository needs `DOCKER_USERNAME`, `DOCKER_PASSWORD`,
`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` and `INFRA_DEPLOY_KEY`.

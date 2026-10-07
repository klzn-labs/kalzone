# Platform

An internal developer platform for the Plateful services, built as platform-engineering
practice. Teams register their services, keep per-environment config and secrets, and
(in later sub-projects) build, release, deploy and watch them from one website.

This repository holds the **control plane**: a Go service with an htmx web UI and a JSON
API on the same usecases. Design documents live in `docs/superpowers/specs`.

## What is in the core

- GitHub sign-in (GitHub App user OAuth), server-side sessions, CSRF protection
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

Never run production with the default secrets key.

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
4. Build a ref from the Releases tab, or switch on auto-build to build every
   push to the default branch.

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

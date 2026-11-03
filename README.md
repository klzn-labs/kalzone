# Kalzone

[![CI](https://github.com/klzn-labs/kalzone/actions/workflows/ci.yml/badge.svg)](https://github.com/klzn-labs/kalzone/actions/workflows/ci.yml)

Kalzone is a small internal developer platform made for AI agents as much as for people.
Teams register a service, and from then on Kalzone builds it, configures it, deploys it
and watches it. An agent does all of that through a typed API and an MCP server, while a
human still approves every change to production.

It runs at https://platform.kalz.one and hosts the Plateful services. This repository is
the control plane: a Go service with an htmx web UI and a JSON API on the same usecases.

## The idea

Most platforms assume a person clicking in a UI or writing YAML. Kalzone assumes the
caller may be Claude Code, a CI job or a scheduled agent, and gives it what it needs to
work safely on its own:

- **Agent identity.** An access token belongs to a person and can do at most what that
  person, its scopes and its teams allow. The audit log records every change as
  "person via token".
- **Human approval for prod.** An agent's change to a protected environment becomes a
  change request that a maintainer approves in the browser.
- **An API agents can drive.** Generated OpenAPI, errors that say how to fix the call,
  `?wait=` for long operations, and readable references such as `latest` or a commit SHA.
- **MCP.** `/mcp` exposes every API operation as a tool and shows each token only the
  tools its scopes allow.

## What it does

- **Builds and releases.** Builds run in GitHub Actions through a GitHub App, with
  registry credentials handed over OIDC. Releases are immutable, digest-pinned images.
- **Config and secrets.** Config is stored per environment as numbered revisions.
  Secrets are encrypted with AES-256-GCM and are write-only.
- **Deploys.** Blue-green or canary rollouts, gated on a smoke test and on Prometheus
  latency and error rates, with automatic rollback. A host agent reports drift, and the
  platform reconciles it.
- **Observability.** Every service gets OpenTelemetry export, logs in Loki and its own
  Grafana dashboard, in a Grafana org per team.
- **Managed resources.** MySQL databases, work queues on NATS JetStream with dead
  letters, and cron jobs.

```
GitHub repo ── build (Actions) ── release ── deployment (release + config) ── dev ──promote──▶ prod
                                                       ▲
                    people (web UI) · agents (API, MCP) ┘
```

## Docs

| Document | Covers |
|---|---|
| [Development](docs/development.md) | layout, running locally, configuration, tests, releasing |
| [Agents](docs/agents.md) | access tokens, the agent API, MCP, approvals |
| [Services](docs/services.md) | onboarding, builds and releases, deleting a service |
| [Diagnostics and telemetry](docs/observability.md) | runtime status, logs, deploy failures, Grafana orgs |
| [Queues](docs/queues.md) | managed queues and the `queued` gateway |
| [Host agent](docs/host-agent.md) | the agent that runs deployments on each host |
| [Plateful on Kalzone](docs/onboarding-plateful.md) | how the first tenant runs on the platform |
| [Design specs](docs/superpowers/specs) | the design behind each feature |

## Quick start

```
docker run -d --name platform-mysql -e MYSQL_ROOT_PASSWORD=test -p 3306:3306 mysql:8
docker exec platform-mysql mysql -uroot -ptest -e "CREATE DATABASE platform"
PLATFORM_GITHUB_CLIENT_ID=... PLATFORM_GITHUB_CLIENT_SECRET=... PLATFORM_ADMINS=your-login make run
```

Needs Go 1.26. See [Development](docs/development.md) for the GitHub App and every
setting.

## License

[MIT](LICENSE)

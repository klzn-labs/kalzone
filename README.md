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
blue-green through infra-plateful's deployctl, like the Plateful apps. The repository
needs the same secrets as go-plateful: `DOCKER_USERNAME`, `DOCKER_PASSWORD`,
`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` and `INFRA_DEPLOY_KEY`.

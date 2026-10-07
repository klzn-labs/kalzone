# Platform core — design

Date: 2026-10-07
Status: draft, awaiting review
Sub-project 1 of 5 of the Plateful Internal Developer Platform.

## 1. Context

Plateful's infrastructure is code (Terraform, compose, deployctl), but delivery is
per-repo GitHub Actions that SSH into the VPS. There is no place where a team can
see what it owns, what runs where, or ask for a change. This project builds an
Internal Developer Platform (IDP) at `platform.kalz.one`, as platform-engineering
practice. Backstage was considered and rejected: the platform is built in-house.

### The whole platform (agreed 2026-10-07)

- Teams register **services**. A service is one container image plus its config.
- The platform **triggers builds** in GitHub Actions (`workflow_dispatch` through a
  GitHub App). A push webhook notifies the platform, which builds if the service
  has auto-build on. The build reports an image digest back; that makes an
  immutable **release**.
- Two fixed **environments**, `dev` and `prod`. Releases are **promoted** dev → prod.
- The **control plane** stores desired state. An **agent** on the host pulls it
  and reconciles Docker to match, embedding deployctl's engine (blue-green,
  Traefik routing, Prometheus gate).
- **Observability is embedded, not rebuilt:** per-service Grafana dashboards
  provisioned by the control plane, logs in Loki (fed by the otel-collector),
  traces in Tempo.
- GitHub sign-in, teams managed in the platform, roles, audit log.
- htmx server-rendered UI on top of a JSON API.

Sub-projects, in order, one spec each:

1. **Core** (this document)
2. Releases — GitHub App webhooks and dispatch, `platform-build.yml`, release lifecycle
3. Agent and deploys — desired state, reconcile loop, deploy, promote, rollback
4. Observability — Loki, provisioned Grafana dashboards, embedding
5. Onboard Plateful — move api/worker/importer/frontend, add the dev environment

## 2. Goals and non-goals

**Goals of core**

- A running control plane at `platform.kalz.one` with GitHub sign-in.
- Teams and memberships with roles; platform admins.
- A service catalog: services owned by teams, with fixed environments.
- Per-service, per-environment config and secrets, versioned as immutable
  config revisions.
- One authorization policy that every usecase goes through.
- An audit log of every change.
- A pretty UI (look chosen from mockups before implementation) and a JSON API
  over the same usecases.

**Non-goals of core**

Builds, releases, the agent, deploys, Grafana embedding, API tokens, the CLI,
GitHub-org team sync, onboarding Plateful. Core deploys nothing.

## 3. Architecture

```
browser ──► controlplane (Go, one binary)
             web (htmx pages)   api (/api/v1, JSON)
                    └──────┬──────┘
             usecases per domain + authz policy
             identity · team · catalog · config · audit
                           │
                 MySQL database `platform`
```

- New repository `github.com/adrianozp/platform`, Go module
  `github.com/adrianozp/platform`. The repo will later also hold the `agent`
  binary.
- Layout and wiring follow the gaardrail standard used by go-plateful: Uber fx,
  viper, Gin, ports and adapters, mockery unit tests.
- Each domain is a package with its own entities, usecases, repository port and
  MySQL adapter, and owns its own tables. **No foreign keys or joins across
  domains.** Cross-domain reads go through interfaces defined by the consumer,
  backed by adapters that call the other domain's usecases, with ids and small
  values only. Cross-domain reactions are events (in-process publisher now).
  These are the same rules as go-plateful's "Cross-Domain Calls".
- `api` serves each domain under its own prefix. `web` is a page layer that
  composes usecases from several domains into one page. It holds no business
  rules.
- State lives in a new `platform` database in the existing Plateful MySQL, with
  its own user, and is covered by the existing backups. Migrations apply on
  startup, as in go-plateful.

## 4. Domains

### 4.1 identity

- `User`: id, github_id (unique), login, name, avatar_url, is_admin, created_at,
  last_login_at.
- `Session`: sha256 of a random 32-byte token, user_id, created_at, expires_at.
  The cookie holds the token; the database holds only its hash.
- Sign-in uses the **GitHub App's user OAuth flow** (the same App that
  sub-project 2 extends with webhooks and dispatch). Flow: `/auth/github` →
  GitHub with a random `state` stored in a short-lived cookie → `/auth/github/callback`
  checks `state`, exchanges the code, reads the GitHub user, upserts `User`,
  creates a `Session`.
- Logins listed in `PLATFORM_ADMINS` get `is_admin = true` on every sign-in.
  This is how the first admin exists. Admins can also grant admin in the UI.
- Any GitHub user can sign in. Without a team membership they can only view.
- Session lifetime 7 days, sliding: renewed when less than 1 day is left.

### 4.2 team

- `Team`: id, slug, name, created_at.
- `Membership`: team_id, user_id, role (`member` | `maintainer`), created_at.
- A `TeamSource` port provides memberships to the rest of the system. Core ships
  the local adapter (memberships stored in the platform). A GitHub-org sync
  adapter can replace it later without a data migration of the other domains.
- A team must always keep at least one maintainer, unless an admin removes the
  last one.

### 4.3 catalog

- `Environment`: id, slug, name, protected, position. Seeded by migration with
  `dev` (not protected) and `prod` (protected). Not editable in the UI.
- `Service`: id, slug, name, description, owner_team_id, github_repo
  (`owner/name`, optional), image_repository (for example
  `docker.io/adrianozdp/plateful-api`), created_at.
- A service has exactly one owning team. Ownership can be moved by a maintainer
  of the current owner or by an admin.
- Deleting a service is a soft delete in core (hidden, slug kept reserved),
  because later sub-projects will have releases and running containers that
  must be torn down first.

### 4.4 config

- `ConfigRevision`: id, service_id, environment_id, number (1, 2, 3… per service
  and environment), spec, env, secrets, author_user_id, message, created_at.
  Immutable once written. The latest revision is the current config.
  - `spec`: port, health_path, host, memory_mb, cpu_millis, replicas.
  - `env`: map of plain variable name → value.
  - `secrets`: map of secret name → secret version id.
- `SecretVersion`: id, service_id, environment_id, name, ciphertext, nonce,
  key_id, created_by, created_at. Immutable.
- Editing config or setting a secret writes a new revision that copies the
  previous one and applies the change. Because revisions point at secret
  *versions*, an old revision still means exactly the secrets it had. In
  sub-project 3, a deploy is release + config revision, so rolling back covers
  config and secrets too.
- A new environment config starts from defaults: port 8080, health_path
  `/healthz`, memory 256 MB, cpu 250 millicores, replicas 1, no host.
- Validation:
  - variable and secret names match `^[A-Z_][A-Z0-9_]*$`, and a name can't be
    both a variable and a secret;
  - `host` must end with one of `PLATFORM_ALLOWED_DOMAINS` and be unique across
    all services and environments;
  - memory 32–4096 MB, cpu 50–4000 millicores, replicas 1–5.

### 4.5 audit

- `AuditEvent`: id, at, actor_user_id, action, target_kind, target_id,
  environment (nullable), details (JSON).
- Usecases publish domain events (`ServiceCreated`, `ConfigRevised`,
  `MembershipChanged`, …). The audit domain subscribes and records them. No
  other domain writes audit rows.
- Details never contain secret values: a secret change records only its name
  and the new version id.
- Read-only. Filterable by team, service, environment, actor.

## 5. Secrets

- AES-256-GCM with a random nonce per value. The key comes from
  `PLATFORM_SECRETS_KEY` (32 bytes, base64), stored in infra-plateful's
  Terraform secrets like the other app secrets.
- Every ciphertext stores a `key_id`, so a later key rotation can re-encrypt
  without guessing which key was used. Core ships one key.
- Write-only: the UI and API show a secret's name, who set it and when, never
  the value. Only the agent will receive decrypted values (sub-project 3).
- The service name, environment and secret name are bound into the ciphertext
  as GCM additional data, so a ciphertext copied to another row fails to decrypt.

## 6. Authorization

One policy, `authz.Can(user, action, resource) error`. Every usecase calls it
first; handlers never decide. It reads memberships through the `TeamSource`
port and a service's owner through a catalog reader.

| Action | signed in | team member | team maintainer | admin |
|---|---|---|---|---|
| View catalog, teams, config (no secret values), audit | ✓ | ✓ | ✓ | ✓ |
| Create a service owned by own team | | ✓ | ✓ | ✓ |
| Edit `dev` config, set `dev` secrets | | ✓ | ✓ | ✓ |
| Edit config and secrets of a protected env (`prod`) | | | ✓ | ✓ |
| Delete a service, move its ownership | | | ✓ | ✓ |
| Add or remove team members, change their role | | | ✓ | ✓ |
| Create or delete teams, grant or revoke admin | | | | ✓ |
| Deploy to `dev` *(enforced in sub-project 3)* | | ✓ | ✓ | ✓ |
| Deploy to `prod` *(enforced in sub-project 3)* | | | ✓ | ✓ |

"Team" means the team that owns the service in question. A denial returns 403
in the API and an inline message in the UI, and is not audited.

## 7. Surface

### 7.1 Web

Pages:

- `/` — your teams and their services, recent activity
- `/services`, `/services/new`, `/services/{service}` — catalog and service page,
  one tab per environment showing current config revision, variables, secret
  names, revision history with diffs
- `/teams`, `/teams/{team}` — teams and members
- `/audit` — audit log with filters
- `/login`, `/logout`

Visual design is decided **before** any UI code: artifact mockups of the look,
the service page and the config editor, for the user to choose from. Then
implemented with server-rendered templates and htmx. Assets are embedded in the
binary.

### 7.2 JSON API (`/api/v1`)

Session cookie auth only in core.

- `GET /me`
- `GET|POST /teams`, `GET|DELETE /teams/{team}`,
  `PUT|DELETE /teams/{team}/members/{login}`
- `GET|POST /services`, `GET|PATCH|DELETE /services/{service}`
- `GET /environments`
- `GET /config/{service}/{env}` (current revision),
  `GET /config/{service}/{env}/revisions`,
  `POST /config/{service}/{env}/revisions` (new revision from a change set),
  `PUT|DELETE /config/{service}/{env}/secrets/{name}`
- `GET /audit`

Errors are JSON `{ "error": { "code", "message" } }` with 400/401/403/404/409.

### 7.3 Request safety

- Session cookie: `HttpOnly`, `Secure`, `SameSite=Lax`.
- Every state-changing request carries a CSRF token (htmx sends it as a header;
  the token is bound to the session).
- Request logs and traces never include cookies, secret values or OAuth codes.

## 8. Configuration

| Key | Meaning |
|---|---|
| `PLATFORM_HTTP_ADDR` | listen address, default `:8080` |
| `PLATFORM_BASE_URL` | `https://platform.kalz.one`, used for the OAuth callback |
| `PLATFORM_DB_DSN` | MySQL DSN for the `platform` database |
| `PLATFORM_GITHUB_CLIENT_ID`, `PLATFORM_GITHUB_CLIENT_SECRET` | GitHub App user-auth credentials |
| `PLATFORM_ADMINS` | comma-separated GitHub logins that are always admin |
| `PLATFORM_SECRETS_KEY` | base64 32-byte key for secrets |
| `PLATFORM_ALLOWED_DOMAINS` | comma-separated host suffixes, e.g. `kalz.one` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | traces to the existing otel-collector |

## 9. Hosting and delivery

- **Image** `adrianozdp/platform-controlplane` on Docker Hub, built by the
  platform repo's CI (test, build, push on main).
- **infra-plateful** adds: the `platform` database and user in MySQL, the
  controlplane service (compose, memory limit 128 MB), its deployctl service
  file so it deploys blue-green like the other apps, a Traefik route for
  `platform.kalz.one`, the Porkbun DNS record in Terraform, and the new secrets
  in tfvars. The platform does not deploy itself.
- **Manual step (user):** create the GitHub App under `adrianozp` with callback
  URLs `https://platform.kalz.one/auth/github/callback` and
  `http://localhost:8080/auth/github/callback`, no repository permissions yet,
  and put its client id and secret in tfvars.
- The controlplane exposes `/healthz` and Prometheus metrics, so the existing
  deploy gate and Grafana work for it.

## 10. Testing

- Unit tests per usecase with mockery mocks.
- A table-driven test covering every cell of the authz matrix.
- Secret tests: encrypt → decrypt round trip, and that a ciphertext moved to
  another service, environment or name fails to decrypt.
- Config revision tests: a change copies the previous revision, older
  revisions stay unchanged, validation rejects each rule in §4.4.
- Integration tests for API handlers against a real MySQL, including sign-in
  with a fake GitHub OAuth server, CSRF rejection and 403s.

## 11. Work items and PRs

Platform repo, one stack of layered commits per the user's rules:

1. Skeleton: module, fx wiring, config, migrations, healthz, CI.
2. UI mockups (artifact) → user picks the look. No code until chosen.
3. Storage: tables and MySQL adapters for all five domains.
4. Rules: usecases, authz policy, secrets crypto, events → audit.
5. Handlers: API, web pages, sign-in, CSRF, with integration tests.

infra-plateful: one PR for database, compose service, route, DNS and secrets.

As with the Kubernetes work, the PRs are opened for the user to read and are not
merged by Claude.

# Platform releases — design

Date: 2026-10-07
Status: self-approved (user asleep, asked Claude to decide), open for review in the PR
Sub-project 2 of 5. Builds on the core (`2026-10-07-core-design.md`).

## 1. Goal

A **release** is an immutable, digest-pinned container image of one service. The
platform decides when a build happens and records the result; GitHub Actions only
builds. Agreed with the user on 2026-10-07:

- CI builds, the platform triggers: `workflow_dispatch` through a GitHub App.
- A push to the default branch notifies the platform (webhook); it builds only if
  the service has **auto-build** on. Manual builds of any ref work too.
- Third-party images (Grafana, Redis) are registered by tag; the platform resolves
  the digest. No build.

Deploying releases is sub-project 3. Nothing here starts containers.

## 2. Flow

```
            ┌───────── push webhook (auto-build on?) ─────────┐
            │                                                 │
 person ──► platform: Request(service, ref) ──► release #n  requested
                │  GitHub App installation token
                ▼
         POST /repos/{repo}/actions/workflows/{workflow}/dispatches
                inputs: release_id, image, ref
                │
                ▼                                  release #n  building
         GitHub Actions (team repo) uses adrianozp/platform/.github/workflows/build.yml
                builds, pushes image:release-<n>-<sha>, reads the digest
                │  OIDC token (aud = platform URL)
                ▼
         POST /api/v1/releases/{id}/built {digest, commit}  ──► release #n  built
                                                           or  failed (report / workflow_run / timeout)
```

## 3. Model

`releases` table, owned by a new **release** domain:

| Column | Notes |
|---|---|
| id, service_id, number | number counts per service: r1, r2… |
| source | `build` or `registered` |
| git_ref, commit_sha | the ref asked for, the commit actually built |
| image | repository, e.g. `docker.io/adrianozdp/plateful-backend` |
| tag, digest | tag pushed or registered; `sha256:…` once known |
| status | `requested` → `building` → `built` \| `failed` |
| trigger | `manual`, `push` or `register` |
| requested_by | user id; empty for push-triggered builds |
| run_url, failure | link to the workflow run; why it failed |
| created_at, finished_at | |

Rules:

- `built` and `failed` are final. A release never changes after it is final,
  and only `built` releases can be deployed (sub-project 3).
- The image reference for deploys is always `image@digest`.
- A service gets two new settings in the catalog: `auto_build` (default off) and
  `build_workflow` (default `platform-build.yml`). Building needs `github_repo`
  and `image_repository` to be set.

## 4. GitHub App

The App created in the core for sign-in gains, at install time:

- repository permission **Actions: read and write** (dispatch, read runs),
- events **push** and **workflow_run**, sent to `https://platform.<domain>/webhooks/github`
  with a webhook secret.

The platform authenticates as the App with an RS256 JWT signed by the App's private
key, exchanges it for an installation token for the repository
(`GET /repos/{owner}/{repo}/installation`, then
`POST /app/installations/{id}/access_tokens`), and caches the token until a minute
before it expires.

## 5. Callback authentication: GitHub OIDC

The build workflow asks GitHub for an OIDC token (`permissions: id-token: write`)
with the platform's URL as audience and sends it as a bearer token. The platform:

1. fetches GitHub's signing keys from
   `https://token.actions.githubusercontent.com/.well-known/jwks` (cached),
2. verifies the RS256 signature, `iss`, `aud`, `exp` and `nbf`,
3. requires the token's `repository` claim to equal the service's `github_repo`,
4. requires the release to be `requested` or `building`.

No shared secret is stored in team repositories. This is the same mechanism cloud
providers use for keyless deploys from GitHub Actions.

## 6. Webhooks

`POST /webhooks/github`, outside the session guard:

- verifies `X-Hub-Signature-256` (HMAC-SHA256 of the body with the webhook secret,
  constant-time compare); anything else is 401,
- `push` to the repository's default branch: for every service with that
  `github_repo` and `auto_build` on, request a release of the pushed commit,
- `workflow_run` `completed` with a non-success conclusion: if a release of that
  service is still `building` and its run matches, mark it `failed` with the
  conclusion,
- other events: 204 and ignored.

A sweeper marks releases still `requested` or `building` after 60 minutes as
`failed: timed out`.

## 7. Registering an image

`Register(service, tag)` resolves the digest with the OCI distribution API:
`HEAD /v2/<name>/manifests/<tag>` with the manifest-list and OCI index media types;
on `401` it follows the `WWW-Authenticate: Bearer realm=…,service=…,scope=…`
challenge for an anonymous pull token. `docker.io/x` maps to
`registry-1.docker.io/v2/x`, and one-segment names get `library/`. The release is
created `built` at once.

## 8. Permissions

New action `release.create`: team **member** (same as editing dev). Push-triggered
builds act as the platform, not a person; their audit actor is `github`.

## 9. Surface

- API: `GET /api/v1/releases?service=`, `POST /api/v1/releases` `{service, ref}`,
  `POST /api/v1/releases/register` `{service, tag}`, `GET /api/v1/releases/{id}`,
  `POST /api/v1/releases/{id}/built` (OIDC), `POST /api/v1/releases/{id}/failed` (OIDC).
- Web: a **Releases** tab on the service page with status badges, a "Build a ref"
  form, a "Register a tag" form, and the auto-build switch with what it needs.
- Reusable workflow `.github/workflows/build.yml` in this repo that team repos call
  from their `platform-build.yml`.

## 10. Configuration

| Key | Meaning |
|---|---|
| `PLATFORM_GITHUB_APP_ID` | numeric App id |
| `PLATFORM_GITHUB_APP_PRIVATE_KEY` | PEM, newlines allowed |
| `PLATFORM_GITHUB_WEBHOOK_SECRET` | webhook HMAC secret |
| `PLATFORM_OIDC_ISSUER` | default `https://token.actions.githubusercontent.com` |
| `PLATFORM_BUILD_TIMEOUT` | default `60m` |

Without an App id the platform still runs; requesting a build answers that builds
are not configured.

## 11. Testing

- JWT signing for the App and OIDC verification against a fake issuer with its own
  key pair: wrong audience, expired, other repository, bad signature all refused.
- Webhook HMAC: valid, tampered body, missing header.
- Registry resolution against a fake registry that demands a bearer token.
- Usecases: lifecycle transitions, auto-build only on the default branch and only
  when on, final states never change, numbering per service.
- Integration: request → dispatch seen by a fake GitHub → OIDC callback → `built`
  and visible in the API and the Releases tab.

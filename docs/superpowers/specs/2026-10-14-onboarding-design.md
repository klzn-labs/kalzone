# Verifiable onboarding — design

Date: 2026-10-14
Status: approved in conversation (decisions of 2026-10-12; on 2026-10-14 the user chose the platform-held registry token and created the Docker Hub token), open for review
Sub-project 6 of 7 of "a platform made for AI agents". Builds on sub-projects 1–5, all live.

## 1. Why

Onboarding a service today needs steps an agent cannot check or do: the GitHub App
must be installed on the repository, the repository needs a `platform-build.yml`
that calls the platform's reusable workflow, it needs `DOCKER_USERNAME` and
`DOCKER_PASSWORD` secrets, and the image must ship `wget` because the container's
health check runs `wget` inside it. When one of these is missing, the first build or
deploy fails and the agent learns why only from a GitHub run or a VPS log
(`hello` on 2026-10-12: no secrets, then no `wget`).

**Done when**, on production:

- `hello` builds through the platform with its `DOCKER_*` secrets removed: the
  build gets push credentials from the platform;
- `getOnboarding` for `hello` reports every check `ok`;
- `createOnboardingPullRequest` for a new test repository opens a pull request with
  a working `platform-build.yml`, after which `getOnboarding` passes and a build
  succeeds;
- a deploy of an image without `wget` passes its health check, and a service that
  never answers its health path fails with class `unhealthy`.

## 2. Decisions

| # | Topic | Decision |
|---|---|---|
| 1 | Registry credentials | The platform holds one Docker Hub token (`PLATFORM_REGISTRY_USERNAME`, `PLATFORM_REGISTRY_TOKEN`) and hands it to a build that proves itself with its GitHub Actions OIDC token. Repositories no longer need `DOCKER_*`. |
| 2 | Who gets it | Only a token whose `repository` is the service's repository, whose `job_workflow_ref` is the platform's reusable build workflow, for a release of that service that is `requested` or `building`. |
| 3 | Onboarding check | `getOnboarding` runs the checks live and reports each with a status, detail and hint. Nothing is cached. |
| 4 | Fixing the repository | `createOnboardingPullRequest` opens a pull request adding `.github/workflows/platform-build.yml`. The platform never writes secrets or pushes to the default branch. |
| 5 | GitHub App | Installed on **All repositories** (user action), with Contents, Pull requests and Workflows read and write (user action). Without them the checks say so in their hints. |
| 6 | Health check | No health check inside the container. deployctl probes `http://<container ip>:<port><health_path>` from the host after the container starts, until it answers 2xx (60 s). Routed services are then smoke-checked through Traefik as today. |
| 7 | New failure step | deployctl exits **14** when the probe never succeeds; the agent classifies it `unhealthy`. |

## 3. Registry credentials

**Endpoint.** `POST /api/v1/releases/:id/registry-credentials` on the public router,
authenticated like the build callbacks (GitHub OIDC bearer, audience = base URL), and
listed with them as the routes outside the API document.

Checks, in order, each a `403 err_forbidden` with a hint unless noted:

1. release exists (`404`);
2. release status is `requested` or `building`;
3. `claims.repository` equals the release's service `github_repo`;
4. `claims.job_workflow_ref` starts with `PLATFORM_BUILD_WORKFLOW` + `@`
   (default `adrianozp/platform/.github/workflows/build.yml`);
5. credentials configured (`503 err_not_configured` otherwise).

Answer `200 {"registry": "docker.io", "username": "...", "password": "..."}`.
An audit event `registry.credentials_issued` names the release, the repository and
the run. The `OIDC Claims` gain `job_workflow_ref`.

**Reusable workflow** (`.github/workflows/build.yml`): a step fetches the credentials
with the OIDC token, masks the password, and runs `docker/login-action` with them.
The `DOCKER_*` secrets become optional and unused, so existing callers keep working.

**Config.** `PLATFORM_REGISTRY_USERNAME`, `PLATFORM_REGISTRY_TOKEN`,
`PLATFORM_BUILD_WORKFLOW`; infra-plateful passes `platform_registry_username` and
`platform_registry_token` from tfvars.

## 4. Onboarding checks

`GET /api/v1/services/:service/onboarding` — `getOnboarding`, scope `read`:

```json
{"service": "hello", "ready": true, "checks": [
  {"name": "github_app", "status": "ok", "detail": "…", "hint": ""},
  …
]}
```

| Check | ok when | Hint when not |
|---|---|---|
| `github_app` | the App's installation covers the repository (`GET /repos/{repo}/installation`) | install the GitHub App on the repository, or set it to All repositories |
| `workflow` | `.github/workflows/<build_workflow>` exists on the default branch and calls `adrianozp/platform/.github/workflows/build.yml` | call createOnboardingPullRequest, or add the file by hand |
| `dockerfile` | `Dockerfile` exists at the repository root | add a Dockerfile at the root |
| `registry` | the platform's credentials may push to `image_repository` (registry token with `push` on that repository) | create the repository on Docker Hub under the platform's account, or set image_repository |
| `config` | the current `dev` config (the default spec counts) has `port` and `health_path` | write a dev config with port and health_path |

`status` is `ok`, `failed`, or `skipped` (a check that depends on a failed one, e.g.
`workflow` and `dockerfile` when the App cannot see the repository). `ready` is
true when every check is `ok`.

`POST /api/v1/services/:service/onboarding/pull-request` —
`createOnboardingPullRequest`, scope `services`, action `service.edit`:

- refuses with `409` when `workflow` is already `ok`;
- creates the branch `platform-onboarding` from the default branch (reuses it if it
  exists), writes `.github/workflows/<build_workflow>` from the template below and
  opens a pull request titled "Build with the platform", or returns the open one;
- answers `201 {"url", "number", "branch"}`;
- GitHub refusing for lack of permission → `502 err_upstream` with hint "grant the
  GitHub App Contents, Pull requests and Workflows write, and accept them on the
  installation".

Template (the same as `hello`'s, without `secrets: inherit`):

```yaml
name: Platform build
run-name: platform release ${{ inputs.release_id }}
on:
  workflow_dispatch:
    inputs:
      release_id: {required: true}
      image: {required: true}
      platform_url: {required: true}
permissions:
  contents: read
  id-token: write
jobs:
  build:
    uses: adrianozp/platform/.github/workflows/build.yml@main
    with:
      release_id: ${{ inputs.release_id }}
      image: ${{ inputs.image }}
      platform_url: ${{ inputs.platform_url }}
```

## 5. Health check from outside (infra-plateful deployctl)

- `stack/compose.app.yml` and `stack/compose.worker.yml` drop `healthcheck`.
  `compose up --wait` then waits for the container to be running.
- A new step `ready` after `Start`, for every service with a `health_path`:
  `docker inspect` the container's IP on the `plateful` network and probe
  `http://<ip>:<port><health_path>` every 2 s for up to 60 s; 2xx passes. A failure
  is `StepReady`, exit code **14**, message `ready <container>: <last error>`; the
  rollback stops the candidate as today.
- Routed services keep the Traefik smoke check after `ready`.
- An image with its own `HEALTHCHECK` keeps it; `--wait` still honours it.

Agent: exit 14 → `unhealthy` (before the exit 10 rules). The `unhealthy` hint changes
to "the container never answered 2xx on health_path: read failure.logs and the
deployment log".

## 6. Testing

- deployctl: `ready` passes on 2xx, fails after the timeout with `StepReady`; exit 14;
  compose files have no `healthcheck` (test reads them).
- platform unit: credential rules (status, repository, workflow ref, configured);
  each onboarding check against a fake GitHub and fake registry auth; template
  content; pull request reuse.
- integration: a build with a valid OIDC token gets credentials, a wrong repository
  or workflow ref gets `403`, a built release gets `403`; `getOnboarding` on
  fakegithub with every check ok and with each failure; `createOnboardingPullRequest`
  creates branch, file and PR on fakegithub; MCP lists both tools; route parity
  treats the new callback like the others.
- agent: exit 14 classifies `unhealthy`.

## 7. Delivery

1. infra-plateful PR: deployctl `ready` step + compose without health checks +
   tfvars/compose plumbing for the registry credentials.
2. platform PR: layers as usual; README "Onboarding a service".
3. Set `platform_registry_username`/`platform_registry_token` in tfvars, apply.
4. Release the platform (installs deployctl), roll the agent image.
5. hello: drop `secrets: inherit` and the repository's `DOCKER_*` secrets; build.
6. After the user grants the App permissions: test repository, PR, build.

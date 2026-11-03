# Services: onboarding, builds and removal

## Onboarding a service

- `GET /api/v1/services/:service/onboarding` (`getOnboarding`) checks what the
  service still needs and answers each check with `status` (`ok`, `failed`,
  `skipped`), a detail and a hint: `github_app` (the GitHub App sees the
  repository), `workflow` (`.github/workflows/<build_workflow>` on the default branch
  calls `klzn-labs/kalzone-actions/.github/workflows/build.yml`), `dockerfile`, `registry`
  (the platform's credentials can push to `image_repository`) and `config` (dev has a
  port and a health path). `ready` is true when all pass.
- `POST /api/v1/services/:service/onboarding/pull-request`
  (`createOnboardingPullRequest`, scope `services`) opens, or returns the open, pull
  request on branch `platform-onboarding` that adds the build workflow. The GitHub
  App needs Contents, Pull requests and Workflows write for it.
- Builds need no Docker secrets: the reusable workflow asks
  `POST /api/v1/releases/:id/registry-credentials` with its GitHub OIDC token, and
  the platform answers its registry credentials (`PLATFORM_REGISTRY_USERNAME`,
  `PLATFORM_REGISTRY_TOKEN`) only to the service's own repository, running
  `PLATFORM_BUILD_WORKFLOW`, while the release is being built. Each hand-out is
  audited as `registry.credentials_issued`.
- Images need no `wget`: deployctl checks `http://<container ip>:<port><health_path>`
  from the host until it answers 2xx (60s), then smoke-checks routed services
  through Traefik. A container that never answers fails with exit 14, class
  `unhealthy`.

## Builds and releases

1. Give the GitHub App the repository permission **Actions: read and write**,
   subscribe it to **push** and **workflow run** events with the webhook URL
   `https://platform.<domain>/webhooks/github`, and install it on the service's
   repository.
2. Add `.github/workflows/platform-build.yml` to the service's repository. The
   Releases tab of every service shows the file; it calls the public
   [`build.yml`](https://github.com/klzn-labs/kalzone-actions/blob/main/.github/workflows/build.yml)
   in klzn-labs/kalzone-actions, which builds and pushes the image with registry
   credentials Kalzone hands it over OIDC and reports the digest. The repository
   holds no Kalzone or registry secret.
   The build always passes `VERSION=<commit>`; more Docker build args go in the
   `build_args` input, one `NAME=value` per line, for example
   `build_args: VITE_GOOGLE_CLIENT_ID=${{ vars.GOOGLE_CLIENT_ID }}`.
3. Build a ref from the Releases tab, or switch on auto-build to build every
   push to the default branch.
4. Switch on auto-deploy (`auto_deploy_dev`) to deploy each release built from a
   push, and each release shared from one, to dev as soon as it is built. Manual
   builds and registered tags are never deployed by themselves, and prod still goes
   through promotion. If a deployment is already running on dev, that build is not
   deployed; deploy it by hand.

## Deleting a service

`DELETE /api/v1/services/:service` (scope `services:admin`) starts a teardown and
answers `202` with the service, `deleted`, and one `teardown` step per
environment the service ever ran in (`removed`, `removing`, or
`awaiting_approval` with its `approve_url`).

- The service becomes `decommissioning` and is frozen: deployments, config and
  secrets, builds, tag registrations, auto-build, auto-deploy, edits and
  transfers are refused. Pending deployments and change requests are cancelled
  and unfinished builds fail. A running deployment makes the call `409`; call
  again once it ends.
- Each environment gets a removal: a deployment with strategy `remove` that the
  agent runs as `deployctl remove`, which takes the route down first, then the
  containers, their images, the deployctl state and metrics; the agent deletes
  the workload's compose and env files. It is gated like a deploy, so a token
  without bypass turns a prod removal into a change request.
- When every removal has succeeded, the queues the service consumes are
  deleted, it is removed from other queues' publishers, its queue tokens are
  revoked, its runtime rows and Grafana dashboard are dropped, and it is marked
  deleted. Its slug is then free for a new service; history stays in the audit
  log.
- Calling `DELETE` again resumes a teardown: removals already running or done
  are left alone and failed ones are scheduled again.

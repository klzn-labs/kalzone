# Onboarding Plateful onto the platform

Sub-project 5 of 5. Everything before this is code; this is the order of operations
to move Plateful's four workloads from GitHub Actions + deployctl onto the platform,
and the gaps found while mapping them.

## 1. Bring the platform up

1. Create the GitHub App under `adrianozp`: user OAuth callback
   `https://platform.kalz.one/auth/github/callback`; repository permission
   **Actions: read and write**; events **push** and **workflow run** to
   `https://platform.kalz.one/webhooks/github` with a secret. Install it on
   `go-plateful` and `fe-plateful`.
2. In this repository: Settings → Actions → General → Access → allow repositories
   owned by `adrianozp`, so the app repositories can call `build.yml`.
3. Fill the tfvars from infra-plateful#10 and #11 (`platform_*`), apply, then run
   this repository's `Release`. Set `platform_agent_image` to the agent image of
   that release and apply again.
4. Sign in; you are admin through `PLATFORM_ADMINS`. Create the teams (for example
   `platform`, `places`, `reviews`) and add people.

## 2. Register the workloads

| Platform service | Repository | Image | dev | prod |
|---|---|---|---|---|
| `api` | go-plateful | `docker.io/adrianozdp/plateful-backend` | see gap 1 | see gap 1 |
| `worker` | go-plateful | same image | command `./worker` | command `./worker` |
| `importer` | go-plateful | same image | command `./importer` | command `./importer` |
| `web` | fe-plateful | `docker.io/adrianozdp/plateful-frontend` | host `dev.kalz.one` | host `kalz.one` (see gap 3) |

For each environment, copy the variables from `infra-plateful/config/<svc>.env` and
set the secrets that Terraform renders today (`terraform/templates/*.secrets.env.tftpl`).
Point dev at a separate database (`plateful_dev`, created by hand once:
`CREATE DATABASE plateful_dev; GRANT ALL ON plateful_dev.* TO 'plateful'@'%';`).

Merge go-plateful#28 and fe-plateful#24, build a release of each from the Releases
tab, deploy to dev, check the embedded panels and logs, then promote.

## 3. Cut over

The platform's workloads are named `<service>-<env>` (`web-prod`), the current ones
`frontend`, `backend`, `worker`, `importer`. Both can run side by side, so:

1. deploy `worker-prod` and `importer-prod`, then stop the old `worker` and
   `importer` containers (they consume the same NATS stream, so briefly running both
   is safe, as it already is during their blue-green deploys);
2. deploy `web-prod` with the host `kalz.one`, then remove the old `frontend` route
   file from `/opt/plateful/dynamic` so only one router claims the host;
3. retire `release.yml` in both app repositories and delete the old service files
   from `infra-plateful/stack/deploy`.

## 4. Gaps found while mapping

1. **Internal routes.** The backend is not public: the BFF reaches it through
   Traefik's host-local `backend` entrypoint on :8081. The runtime spec only knows a
   public host or a worker. Next step: an optional `entrypoint`/`internal` field in
   the spec that the deployctl runtime turns into `entrypoint: backend` with a
   `PathPrefix(`/`)` rule.
2. **One image, several services.** `api`, `worker` and `importer` share an image,
   so a push with auto-build on would build it three times. Turn auto-build on for
   `api` only and register the same digest for the other two, or add "release
   from another service's build" later.
3. **More than one host.** `web` answers on `kalz.one` and `www.kalz.one`; the spec
   has a single host. Next step: a list of hosts.
4. **Replicas and CPU** are ignored by the compose runtime (see the deploys spec);
   they become real with a Kubernetes runtime.

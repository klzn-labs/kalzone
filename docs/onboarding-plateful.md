# Plateful on the platform

Plateful moved onto the platform on 2026-10-08. All four workloads run from it, and
infra-plateful now deploys only the platform's own control plane.

## What runs where

| Service | Repository | Image | dev | prod |
|---|---|---|---|---|
| `api` | go-plateful | `docker.io/adrianozdp/plateful-backend` | internal, `traefik:8091`, database `plateful_dev` | internal, `traefik:8081` |
| `worker` | go-plateful | same image, shared builds | not deployed | command `./worker` |
| `importer` | go-plateful | same image, shared builds | not deployed | command `./importer` |
| `web` | fe-plateful | `docker.io/adrianozdp/plateful-frontend` | `dev.kalz.one` | `kalz.one` + alias `www.kalz.one` |

All four belong to the team `plateful`. Containers are `<service>-<env>-<colour>`.

`worker` and `importer` have no dev deployment on purpose: they consume the prod
NATS stream, and a dev copy would take messages meant for prod. They need a dev
NATS account and stream before they get a dev environment.

`api` in dev has no object storage secrets, so review uploads fail there. Give it a
separate R2 bucket before testing media in dev.

## Shipping a change

1. Build `api` or `web` from the Releases tab, or switch auto-build on. A build of
   `api` is shared with `worker` and `importer`, which get a release with the same
   digest.
2. Deploy the release to dev and check the dev site and the embedded panels.
3. Promote dev to prod for `api` and `web`. Deploy the shared release of `worker` and
   `importer` to prod directly.

## How the cutover went

For each service: copy the variables from `infra-plateful/config/<svc>.env` and the
secrets from the host into the platform, register the live image tag and deploy it,
move the old Traefik route file out of `/opt/plateful/dynamic`, check from the
container logs that real traffic reaches the new colour and only health checks reach
the old one, then stop and remove the old containers. The old route files and
deployctl state are in `~deploy/retired-state/` on the host.

## Gaps found while mapping

1. **Internal routes.** Closed: a spec can be `internal`, routed on the environment's
   host-local entrypoint (`AGENT_INTERNAL_ENTRYPOINTS`), one service per environment.
2. **One image, several services.** Closed: a finished build is shared with every
   service of the same repository and image, and a push builds each image once.
3. **More than one host.** Closed: a spec takes `aliases`.
4. **Replicas and CPU** are still ignored by the compose runtime (see the deploys
   spec); they become real with a Kubernetes runtime.

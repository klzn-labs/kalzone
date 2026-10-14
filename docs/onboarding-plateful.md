# Plateful on the platform

Plateful moved onto the platform on 2026-10-08. All four workloads run from it, and
infra-plateful now deploys only the platform's own control plane.

## What runs where

| Service | Repository | Image | dev | prod |
|---|---|---|---|---|
| `api` | go-plateful | `docker.io/adrianozdp/plateful-backend` | internal, `traefik:8091`, database `plateful_dev` | internal, `traefik:8081` |
| `worker` | go-plateful | same image, shared builds | command `./worker`, database `plateful_dev` | command `./worker` |
| `importer` | go-plateful | same image, shared builds | command `./importer`, database `plateful_dev`, no Google Places key | command `./importer` |
| `web` | fe-plateful | `docker.io/adrianozdp/plateful-frontend` | `dev.kalz.one` | `kalz.one` + alias `www.kalz.one` |

All four belong to the team `plateful`. Containers are `<service>-<env>-<colour>`.
The importer hands places to the worker through the managed queue `place-import`
(one per environment, consumer `worker`, publisher `importer`).

The live picture is in the API, not in this file: `GET /api/v1/services?team=plateful`,
`GET /api/v1/config/<service>/<env>` for variables and secret names, and
`GET /api/v1/queues?team=plateful`.

## Working with the API

Every call, field, scope and status value is described in
`GET https://platform.kalz.one/api/v1/openapi.json`. Read it before calling
anything; do not guess routes from the source. Send
`Authorization: Bearer <token>`; `GET /api/v1/whoami` shows what the token may do.
Lists come in pages of `{"items", "next_cursor"}`, and a refused call says in
`error.hint` how to fix it.

## Shipping a change

1. Build `api` or `web`: `POST /api/v1/releases` with `{"service", "ref"}`, or the
   Releases tab, or auto-build on push. A build of `api` is shared with `worker` and
   `importer`, which get a release with the same digest and their own number.
2. Wait for it: `GET /api/v1/releases/<id>?wait=120s` answers `200` once it is
   `built` or `failed`, or `202` to call again.
3. Deploy the services to dev in one call:
   `POST /api/v1/deployments?wait=120s` with
   `{"targets": [{"service": "api", "environment": "dev"}, {"service": "worker", "environment": "dev"}, {"service": "importer", "environment": "dev"}], "release": "sha:<commit>"}`;
   the release is resolved per service. Read a failed deployment's `events_summary`
   with `GET /api/v1/deployments/<id>`.
4. Promote to prod: `POST /api/v1/deployments/promotions` with
   `{"service", "from": "dev", "to": "prod"}`, one service per call. A token without
   bypass gets `202` with a change request; a maintainer approves it in the browser,
   and `GET /api/v1/requests/<id>?wait=120s` says `executed` with the deployment.
5. Change config with `PATCH /api/v1/config/<service>/<env>` and
   `Content-Type: application/merge-patch+json`, naming only what changes;
   `GET /api/v1/config/search?key=<NAME>` shows where a variable is set.

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
5. **Workers in dev.** Closed: the NATS stream is gone; worker and importer talk
   through the managed queue, which has its own dev queue.

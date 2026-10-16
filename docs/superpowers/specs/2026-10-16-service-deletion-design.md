# Service deletion

Status: approved, 2026-10-16. Registry cleanup deferred by the user.

## Problem

`DELETE /api/v1/services/:slug` only sets `deleted_at`. Nothing else follows:

- On the host, the containers, the Traefik route, the deployctl state, the env files (secrets included), the pushgateway groups and the local images all stay.
- On the platform, the Grafana dashboard, queues and queue publishers stay. Pending deployments, change requests and unfinished builds go stale.
- The agent keeps polling the dead workloads.
- The slug can never be used again.

Already fixed separately (#27): hosts and the internal slot of deleted services are freed, and a deleted publisher's queue token is dropped.

## Decisions (user, 2026-10-16)

1. Queues the service consumes are deleted with it. Every queue has exactly one consumer, so this covers them all. The service is also removed from the publishers of every other queue.
2. Tearing down prod goes through a change request, like a deploy.
3. The slug becomes reusable once the service is deleted.
4. Registry images are cleaned up when the platform can do it safely, which matters most for a self-hosted registry. **Deferred**: the design below stays for later.
6. The host keeps at most the 3 newest local images per image repository (plus any image a container still uses).
5. The leftovers of `hello` and `lolgrid` were cleaned by hand on 2026-10-16.

## Flow

Deleting is a process that can take a while and can wait on prod approval. It is not a single write.

1. **Request.** A maintainer (or an admin) calls `DELETE /api/v1/services/:slug`.
   - Refused with `409` while a deployment of the service is `in_progress`.
   - Otherwise the service becomes `decommissioning` and the call answers `202` with the service and its teardown deployments.
   - A second `DELETE` returns the same state.
2. **Freeze.** A `decommissioning` service refuses new deployments, builds, config and secret changes, and queue changes (`409`). Auto-build, auto-deploy and build sharing skip it.
3. **Cancel what is in flight:**
   - pending deployments become `cancelled`;
   - pending change requests for the service become `cancelled` with the reason "service deleted";
   - releases still `requested` or `building` become `failed` ("service deleted").
4. **Queues:**
   - Delete every queue the service consumes, as a forced delete (messages are dropped).
   - Write a new version of every queue that lists it as a publisher, without it.
   - Delete its `queue_credentials`.
   - Prod queue deletions are carried by the prod teardown request (step 5), not by separate requests.
5. **Teardown, one per environment the service ever deployed to.** Each is a deployment row with `strategy=remove` and `trigger=decommission`.
   - On dev it is scheduled at once.
   - On prod it is a change request ("remove api from prod"). The prod queue deletions run when it is approved.
   - The agent claims a removal like any deployment and runs `deployctl remove -service <svc>-<env>`, which:
     1. deletes `dynamic/<svc>-<env>.yml`, so traffic stops;
     2. stops and removes both colour containers (`docker compose -p … down`, falling back to `docker rm -f` by name);
     3. removes the images those containers used, without `-f` (an image still used by another container stays);
     4. deletes `state/<svc>-<env>.json`, and the agent deletes `stack/deploy/<svc>-<env>.yml`, `config/<svc>-<env>.env` and `secrets/<svc>-<env>.env`;
     5. deletes the pushgateway groups `plateful_deploy`, `plateful_rollback` and `plateful_traffic` for `<svc>-<env>`;
     6. writes a Grafana annotation "removed".
   - Every step tolerates the thing already being gone, so a retry is safe. The agent streams events and reports Finish as usual.
   - A failed removal leaves the service `decommissioning`, and the service page offers "retry teardown".
6. **Finish**, when every teardown has succeeded:
   - delete the `runtime_statuses` rows;
   - delete the Grafana dashboard `platform-<slug>`;
   - set `deleted_at`, then publish `service.deleted`.

   This step runs from the subscriber that sees the last removal succeed, and once more from the sweeper so that a crash in between cannot strand it.

## Desired state

`AllLive` returns the latest succeeded deployment per service and environment. After a removal succeeds that row is the removal, so `Desired` skips the pair. The workload drops out of the agent's desired list, and the `record not found` polling stops.

The agent never deletes by diffing its desired list against the host. An empty list during a database outage must not mean "delete everything", and the host also runs workloads the platform doesn't manage (the controlplane itself).

## Slug reuse

- Drop the unique key `services_slug`.
- Add a generated column `live_slug = IF(deleted_at IS NULL, slug, NULL)` with a unique key. MySQL allows many NULLs, so deleted rows no longer block the name.
- Everything that names a service by slug only looks at live services. History stays attached to the old service id, and the audit log keeps the slug as text.
- A new service with a reused slug starts with no config, releases or dashboard history.

## Registry cleanup (deferred)

It runs at Finish and never blocks it. The outcome (`skipped: <reason>`, `deleted N tags`, `failed: …`) goes in the audit event.

The platform cleans an image repository only when all of these hold:

- it is in the registry and namespace the platform pushes to (`PLATFORM_REGISTRY_*`), never a third-party image;
- no live or decommissioning service has the same `image_repository`, which protects shared builds such as `plateful-backend`;
- it is not on the denylist (`PLATFORM_REGISTRY_KEEP`, default `adrianozdp/platform-*`).

It then deletes the whole repository's tags. Keeping "the latest" is pointless once no service uses the repository.

Two adapters behind one port:

- **Docker Hub:** `DELETE /v2/namespaces/{ns}/repositories/{repo}`, using a separate personal access token with Read, Write and Delete (`PLATFORM_REGISTRY_DELETE_TOKEN`). That token is never handed to builds; the build token stays Read & Write.
- **OCI distribution (self-hosted registry):** list the tags, resolve each to a digest, then `DELETE /v2/<name>/manifests/<digest>` with the registry credentials. The registry needs deletes enabled; its garbage collection frees the disk.

Without a delete token, Docker Hub cleanup is skipped and says so.

## Host image retention

After every successful deploy, `deployctl deploy` prunes the local images of the deployed image repository:

- It keeps the 3 newest by creation time.
- It removes the rest with `docker image rm`, without `-f`. Docker refuses an image a container still uses, running or stopped, so the live image and the stopped previous colour (the rollback target) always stay.
- A refusal or failure is logged and never fails the deploy.

A one-off run cleans the host for the existing backlog.

## API and UI

- `DELETE /api/v1/services/:slug` answers `202` with `{"service", "status": "decommissioning", "teardown": [deployments or change requests]}`. It needs the `services:admin` scope on tokens. It is not an MCP tool, matching v1, where deleting stays out of the MCP tools.
- `GET /api/v1/services/:slug` keeps answering for a decommissioning service, with `status`.
- The service page shows the teardown progress per environment while decommissioning, and offers retry.
- The delete confirmation lists what will be removed: environments, queues it consumes, and the queues it publishes to.

## Out of scope

- Undeleting a service.
- Purging `secret_versions` and `config_revisions`; they stay under the old service id.
- Host-wide pruning of old images of live services, about 18 GB today. That is a separate host policy, because those images are rollback targets.

## Repositories and order

1. infra-plateful: `deployctl remove` and the image retention after deploy, with tests, released and installed on the host first.
2. platform: storage first (migration with `live_slug`, the service `status` column, removal strategy and trigger).
3. platform: rules (decommission usecase, cancellations, queues, removal scheduling and Finish, `AllLive`, freeze checks).
4. platform: handlers (API, UI, agent `remove` path, wiring) with integration tests.

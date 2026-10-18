# Managed queues — design

Date: 2026-10-08
Status: agreed section by section with the user on 2026-10-08, waiting for review of this document

## 1. Goal

A team clicks **Create queue** in the platform and gets a work queue it configures
(rate limit, retries, backoff, retention, dead letters) without knowing which broker
runs it. NATS JetStream backs every queue today; Google Pub/Sub or others can be added
later as drivers without changing application code.

User decisions, 2026-10-08:

- Apps talk to queues over **HTTP**: they publish with a POST to the platform's queue
  gateway, and the gateway delivers with a POST to an endpoint on the consumer service
  (push). The gateway enforces rate limits, retries and dead letters.
- **Work queues** only: one consumer service per queue, each message processed once.
- The gateway is a **new component, one per host** (`queued`), not part of the control
  plane or the agent, so messages keep flowing while the control plane is down or
  being deployed.
- A **Go library** handles publishing and receiving, in a **new public repository**
  `adrianozp/platform-sdk-go`, so app builds need no credentials.

## 2. Pieces

| Piece | Where | What |
|---|---|---|
| Queue domain | platform, control plane | entities, `queues` table, use cases, API, audit, authorization |
| Gateway | platform, `cmd/queued`, image `adrianozdp/platform-queued` | publish endpoint, one dispatcher per queue, stats, metrics |
| Backend port + JetStream driver | platform, gateway | the only code that knows NATS |
| Network alias | infra-plateful, deployctl compose files | every app container answers on `<service>-<env>` on the `plateful` network |
| Gateway deployment | infra-plateful | `queued` in `compose.platform.yml`, own NATS user, image and token from tfvars |
| Go library | `adrianozp/platform-sdk-go`, module `.../queue` + `.../queue/queuetest` | publish client, delivery handler, test helpers |
| UI | platform, control plane | Queues pages, DLQ view, panels; mockups first |
| Plateful migration | go-plateful | the `place-import` queue replaces direct NATS use between importer and worker |

```
                         control plane (platform.<domain>)
                         queue CRUD, UI, stats, dead letters
                                 ▲ desired state (poll)    │ stats (push)
                                 │                          ▼
importer-prod ──POST──► queued (one per host, network plateful) ──► NATS JetStream
                         │  publish: service token              one stream per queue
                         │  dispatch: rate limit, retries
                         ▼
                http://worker-prod:8081/hooks/place-import   (2xx = ack)
```

## 3. The queue

A queue belongs to one environment: `place-import` in dev and `place-import` in prod
are separate queues with separate streams. A queue belongs to the team that owns its
consumer service.

| Field | Default | Limits |
|---|---|---|
| `name` | — | slug `^[a-z][a-z0-9-]{1,62}$`, unique per environment |
| `consumer` | — | a service of the owning team |
| `path` | — | starts with `/`, at most 255 characters |
| `publishers` | the consumer only | list of services, any team |
| `rate_per_second` | 10 | 0.1–1000 |
| `max_in_flight` | 4 | 1–100 |
| `max_attempts` | 5 | 1–50 |
| `backoff` | exponential, 1s initial, 5min max | `fixed` or `exponential`; initial 100ms–1h; max ≥ initial, ≤ 6h |
| `delivery_timeout` | 30s | 1s–15min |
| `retention` | 7 days | 1h–30 days |
| `max_message_bytes` | 256 KiB | 1 KiB–1 MiB |
| `paused` | false | — |

Every change creates a numbered version of the queue's settings, kept for history and
written to the audit log. The gateway applies a new version without restarting.
Editing queues follows the same policy as editing config: members edit dev,
maintainers and admins edit prod.

A queue cannot be deleted while it holds messages unless the request says `force`.

## 4. Delivery semantics

**At least once.** A message can be delivered more than once (the consumer handled
it but the response was lost, or the gateway restarted mid-delivery), so consumers
must be idempotent.

Each delivery is a `POST` to `http://<consumer>-<env>:<port><path>`, where `<port>` is
the consumer's configured port, with:

- the message body and its original `Content-Type`
- `Platform-Message-Id`, `Platform-Attempt` (1-based), `Platform-Queue`,
  `Platform-Published-At` (RFC 3339)
- `Platform-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(key, t + "." + body)>`

The consumer's response decides what happens:

| Response | Outcome |
|---|---|
| 2xx | ack, the message is done |
| 4xx except 408 and 429 | permanent failure, straight to the dead letters |
| 408, 429, 5xx, timeout, connection refused | retry with backoff; a `Retry-After` header (seconds) replaces the computed delay, capped at the backoff max |
| attempts exhausted | dead letters, with the last status and error |

Backoff: `fixed` waits `initial` every time; `exponential` waits
`min(initial × 2^(attempt-1), max)` with ±20% jitter.

A paused queue accepts publishes and delivers nothing; attempts are not spent while
paused.

## 5. Publishing

`POST http://queued:8080/v1/queues/{name}/messages`

- `Authorization: Bearer <PLATFORM_QUEUE_TOKEN>`; the token identifies the service and
  environment, and the queue is looked up in that environment.
- Body: opaque bytes up to `max_message_bytes`; `Content-Type` is kept.
- Optional `Idempotency-Key` (≤ 128 characters): a second publish with the same key
  within 2 minutes returns the first message's id and stores nothing (JetStream
  message deduplication).
- Optional `Platform-Delay` (seconds, ≤ 24h): the first delivery waits that long.
- `202 {"message_id": "..."}`; `401` bad token, `403` not a publisher, `404` unknown
  queue, `413` too large, `503` backend unavailable.

## 6. Configuration injected into services

For every service and environment the platform resolves, without the team doing
anything:

| Name | Kind | For |
|---|---|---|
| `PLATFORM_QUEUE_URL` | variable | `http://queued:8080` |
| `PLATFORM_QUEUE_TOKEN` | secret | publishing; one token per service and environment |
| `PLATFORM_QUEUE_SIGNING_KEY` | secret | verifying deliveries; one key per service and environment |

They are generated on first use, stored sealed like other secrets, and added to the
resolved config the agent receives. They are reserved names: a team cannot set them by
hand. Rotation regenerates both and needs a redeploy of the service; the gateway
accepts the previous signing key for 10 minutes after a rotation.

## 7. Control plane

**Storage** (migration `000004_queues`): `queues` (identity, environment, owning team,
consumer, path, current version, paused, timestamps), `queue_versions` (settings JSON,
author, message), `queue_publishers`, `queue_credentials` (sealed token and signing key
per service and environment, previous key and rotation time), `queue_commands`
(pending operations for the gateway: replay, replay all, purge dead letters, delete).

**API** (`/api/v1`):

- `GET /queues?environment=&team=`, `POST /queues`
- `GET|PATCH|DELETE /queues/{env}/{name}`
- `POST /queues/{env}/{name}/pause`, `/resume`
- `GET /queues/{env}/{name}/stats`
- `GET /queues/{env}/{name}/dead`, `POST /queues/{env}/{name}/dead/{id}/replay`,
  `POST /queues/{env}/{name}/dead/replay-all`, `DELETE /queues/{env}/{name}/dead`

**Gateway API** (bearer `PLATFORM_QUEUED_TOKEN`, shared like the agent token):

- `GET /queued/v1/desired`: every queue with its current settings, publisher tokens
  (hashed) and signing keys of the consumers, plus pending commands
- `POST /queued/v1/stats`: per queue depth, in flight, oldest message age, delivered
  and failed per minute, dead letters count, and up to 50 newest dead letters
  (id, attempts, last status, last error, published at, first 4 KiB of the body)
- `POST /queued/v1/commands/{id}/done`

The control plane keeps the latest stats in memory per gateway and shows "no data"
when a gateway has not reported for 60 seconds.

## 8. Gateway (`queued`)

- Polls `desired` every 5 seconds; keeps running on the last good copy when the
  control plane is unreachable; reports stats every 15 seconds.
- On a new or changed queue: ensures the stream and the durable consumer through the
  backend port, then starts or reconfigures that queue's dispatcher. On a deleted
  queue: stops the dispatcher and deletes both streams.
- Dispatcher per queue: a token bucket at `rate_per_second`, at most `max_in_flight`
  deliveries at once, an HTTP client with `delivery_timeout`, the outcome table in §4.
- Prometheus metrics on `:9090/metrics`: `platform_queue_depth`,
  `platform_queue_in_flight`, `platform_queue_delivered_total{outcome}`,
  `platform_queue_delivery_seconds`, `platform_queue_dead_letters`, all labelled by
  `queue` and `environment`.

**Backend port**, the only place that knows the broker:

```go
type Backend interface {
    Ensure(ctx context.Context, q Queue) error
    Remove(ctx context.Context, q Queue) error
    Publish(ctx context.Context, q Queue, m Outgoing) (id string, err error)
    Fetch(ctx context.Context, q Queue, max int) ([]Delivery, error)
    Ack(ctx context.Context, d Delivery) error
    Retry(ctx context.Context, d Delivery, after time.Duration) error
    Bury(ctx context.Context, d Delivery, reason DeadReason) error
    Dead(ctx context.Context, q Queue, limit int) ([]DeadLetter, error)
    Replay(ctx context.Context, q Queue, id string) error
    PurgeDead(ctx context.Context, q Queue) error
    Stats(ctx context.Context, q Queue) (BackendStats, error)
}
```

**JetStream driver:** stream `Q_<ENV>_<NAME>` (subject `q.<env>.<name>`, file storage,
`MaxAge` = retention, `MaxMsgSize`, `Duplicates` = 2 minutes); durable pull consumer
`dispatch` with `AckWait` = delivery timeout + 5 seconds and `MaxDeliver` unlimited
(the gateway counts attempts and buries); retries are `NakWithDelay`; dead letters go
to stream `DLQ_<ENV>_<NAME>` with the reason in headers, then the original is acked;
replay republishes to the queue's subject and deletes the dead letter. The gateway
connects as its own NATS user, `queued`.

Attempts are counted from JetStream's delivery count. A delayed publish stores its due
time in a header; when the dispatcher fetches it early, it defers it once with
`NakWithDelay(due - now)` and that delivery does not count as an attempt. A paused
queue's dispatcher stops fetching, so pausing spends no deliveries.

## 9. Infra

- deployctl's `compose.app.yml` adds `aliases: ["${SERVICE}"]` on the `plateful`
  network, so `worker-prod` resolves to whichever colour is running, and to both
  during a blue-green overlap. Workers without a route can receive deliveries.
- `nats.conf` moves from one user to a `users` list: `plateful` (unchanged) and
  `queued` (new password from tfvars).
- `compose.platform.yml` runs `queued` on the `plateful` network with
  `PLATFORM_URL`, `QUEUED_TOKEN`, `QUEUED_NATS_URL/USER/PASSWORD`; tfvars
  `platform_queued_image`, `platform_queued_token`, `nats_queued_password`.
- Prometheus scrapes `queued:9090`.

## 10. Go library (`adrianozp/platform-sdk-go`)

Public repository, module `github.com/klzn-labs/kalzone-sdk-go`, standard library
only, tagged `queue/v0.x.y`.

```go
q, err := queue.FromEnv() // PLATFORM_QUEUE_URL, PLATFORM_QUEUE_TOKEN
id, err := q.Publish(ctx, "place-import", body, queue.WithContentType("text/plain"))
id, err := q.PublishJSON(ctx, "place-import", job,
    queue.WithIdempotencyKey(job.CityID), queue.WithDelay(time.Minute))

h := queue.Handler(os.Getenv("PLATFORM_QUEUE_SIGNING_KEY"),
    func(ctx context.Context, m queue.Message) error {
        var job ImportJob
        if err := m.DecodeJSON(&job); err != nil {
            return queue.Permanent(err) // 422 → dead letters
        }
        return process(ctx, job) // nil → 200, error → 500 → retry
    })
```

- `Publish` retries network errors and 5xx three times (100ms, 400ms, 1.6s), always
  with an idempotency key (generated when the caller gives none). 401, 403, 404 and
  413 come back as `queue.ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`,
  `ErrTooLarge`.
- `Handler` verifies the signature and refuses timestamps older than 5 minutes (401),
  turns `nil` into 200, `queue.Permanent(err)` into 422,
  `queue.RetryAfter(d, err)` into 503 with `Retry-After`, any other error into 500.
  `queue.Message` has `ID`, `Attempt`, `Queue`, `PublishedAt`, `Header`, `Body`.
- `queuetest.NewGateway()` is a fake gateway that records publishes;
  `queuetest.Delivery(key, body, opts...)` builds a signed request for handler tests.
- The README shows how to verify the signature in other languages.

A contract test in the platform repo pins the headers and the signature format
against the library's `queuetest` helpers, so the two sides cannot drift.

## 11. UI

A **Queues** item in the sidebar: queues by team and environment with depth, rate and
dead letters. A page per queue: settings (editable per §3 policy), depth and delivery
charts from the latest stats, the dead letters table (body preview, attempts, last
error, replay, replay all, purge), pause and resume. The service page gets "Consumes"
and "Publishes to" cards. Grafana gets a per-queue row in the consumer service's
dashboard. Mockups are shown as an artifact before the UI is built.

## 12. Errors and failure modes

| Situation | Behaviour |
|---|---|
| Backend down | publish returns 503 and the library retries; dispatchers back off and resume |
| Gateway restarts | nothing is lost: messages and consumer position live in JetStream |
| Consumer down | messages wait and retry with backoff; exhausted ones go to dead letters |
| Control plane down | the gateway keeps publishing and delivering on its last desired state |
| Signing key rotated | previous key accepted for 10 minutes |
| Unknown or deleted queue | publish returns 404 |

Default alerts: dead letters grew in the last 10 minutes; oldest message older than
15 minutes.

## 13. Testing

- Unit: queue validation and versioning, authorization, outcome table, backoff with
  jitter bounds, token bucket, signature (valid, tampered, expired, rotated key).
- Control plane integration (`internal/integration`): CRUD, prod policy, injected
  config and reserved names, desired state, stats, dead letter commands.
- Gateway against a real JetStream (container in tests): publish and deliver, retry
  with backoff, permanent failure, exhausted attempts, replay, pause, restart without
  loss, idempotency key dedupe, `Retry-After`.
- Library: tests in its own repository against `queuetest`; the contract test above.

## 14. Delivery order

Each step is its own set of layered commits, merged and deployed before the next.

1. Control plane: queue domain, migration, API, injected config, audit.
2. Gateway `queued` with the JetStream driver.
3. Infra: network alias, `queued` deployment, NATS users, Prometheus scrape.
4. Library: `platform-sdk-go` with `queue` and `queuetest`, tag `queue/v0.1.0`.
5. UI: mockups, then the pages and Grafana panels.
6. Plateful: queue `place-import` in dev and prod. The worker exposes
   `POST /hooks/place-import` and accepts work from both NATS and the queue; the
   importer switches to publishing through the library; once the old stream is
   drained, the worker's direct NATS consumer is removed. Worker and importer then get
   dev deployments, since the dev queue is separate.

## 15. Out of scope

Topics with fan-out, other backends (only the port exists), queues across hosts,
ordering guarantees, managed databases and other managed resources.

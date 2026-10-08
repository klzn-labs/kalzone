# Managed queues, plan 2 of 6: the `queued` gateway

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `queued` binary that takes publishes over HTTP, keeps messages in NATS JetStream, delivers them with signed POSTs to the consumer under the queue's rate limit, concurrency and retry policy, buries exhausted messages in a dead-letter stream, runs the control plane's commands and reports stats — so plan 3 can deploy it next to the stack.

**Architecture:** `cmd/queued` wires `internal/queued`: a control-plane client (desired, stats, done) over shared DTOs in `pkg/queuedapi`; a `Backend` port with a JetStream driver; one dispatcher per queue; a publish HTTP server; a registry that reconciles dispatchers with the desired state and executes commands; Prometheus metrics. Image target `queued` in the existing Dockerfile; the release workflow pushes `adrianozdp/platform-queued`.

**Tech Stack:** Go 1.26, `github.com/nats-io/nats.go` v1.48 (`jetstream` package), `github.com/nats-io/nats-server/v2` v2.10 embedded in tests, `golang.org/x/time/rate`, `github.com/prometheus/client_golang`, testify.

**Spec:** `docs/superpowers/specs/2026-10-08-managed-queues-design.md` §4, §5, §8, §12. Plan 1 (`2026-10-08-queues-1-control-plane.md`) produced `/queued/v1`.

## Global Constraints

- Function bodies ≤ 20 lines, ≤ 2 nesting levels, no comments; layered commits (storage/backend → rules/dispatch → handlers/wiring/integration); no `Co-Authored-By`.
- Delivery outcome table (spec §4): 2xx ack; 4xx except 408/429 → dead letters; 408, 429, 5xx, timeout, connection error → retry; `Retry-After` seconds replaces the delay, capped at backoff max; attempts exhausted → dead letters.
- Exponential delay `min(initial × 2^(attempt-1), max)` with ±20% jitter; fixed delay `initial`.
- Delivery headers: `Platform-Message-Id`, `Platform-Attempt` (1-based), `Platform-Queue`, `Platform-Published-At` (RFC 3339), `Platform-Signature: t=<unix>,v1=<hex HMAC-SHA256(key, t + "." + body)>`, original `Content-Type`.
- Publish: `POST /v1/queues/{name}/messages`, bearer token matched by SHA-256 against the queue's `publisher_hashes`; `Idempotency-Key` ≤ 128 chars deduplicated for 2 minutes; `Platform-Delay` seconds ≤ 86400; responses 202 `{"message_id"}`, 401, 403, 404, 413, 503.
- Streams: `Q_<ENV>_<NAME>` subject `q.<env>.<name>`; `DLQ_<ENV>_<NAME>` subject `dlq.<env>.<name>`; `QD_<ENV>_<NAME>` subject `qd.<env>.<name>` (delayed); names upper-cased with `-` → `_`.
- Env: `QUEUED_PLATFORM_URL`, `QUEUED_TOKEN` (the control plane's `PLATFORM_QUEUES_TOKEN`), `QUEUED_NATS_URL`, `QUEUED_NATS_USER`, `QUEUED_NATS_PASSWORD`, `QUEUED_ADDR` (`:8080`), `QUEUED_METRICS_ADDR` (`:9090`), `QUEUED_POLL` (`5s`), `QUEUED_STATS_EVERY` (`15s`).
- Carry-overs from plan 1's review: reconcile on `paused` as well as `version`; a queue that disappears from desired state **without** a `delete` command stops its dispatcher but keeps its streams.

## Rulings made while planning

- **Delayed messages use a delay stream**, not the spec's "defer once with `NakWithDelay`": a deferral would be counted by JetStream's delivery count, making `Platform-Attempt` ambiguous. A per-queue mover fetches `QD_…`, `NakWithDelay`s messages not yet due and republishes due ones to `q.<env>.<name>` with the original message id. Cost if wrong: one extra stream per queue.
- **Attempts = JetStream `NumDelivered`** on the main stream; `MaxDeliver` unlimited; the gateway buries at `max_attempts`.
- **Shared DTOs** move to `pkg/queuedapi`, and plan 1's `gatewayapi` handler is switched to them so client and server cannot drift (like `pkg/agentapi`).
- This plan lists interfaces, behaviours and tests per task; code is written test-first during execution (user: "just go for it").

## Review Focus

1. A consumer that hangs past `delivery_timeout`: the delivery is cut and retried; JetStream must not redeliver it in parallel (`AckWait` > timeout). Test in Task 3.
2. Desired state changes `max_in_flight` or rate while deliveries are in flight: the dispatcher reconfigures without dropping or duplicating in-flight acks. Test in Task 4.
3. The control plane is unreachable at gateway start: the gateway serves no queues but stays up, and retries. Test in Task 4.
4. A publish for a paused queue is accepted and delivered after resume. Test in Task 5.
5. Gateway restart mid-delivery: unacked messages are redelivered after `AckWait`, nothing lost. Test in Task 3.

## File Structure

| File | Responsibility |
|---|---|
| `pkg/queuedapi/queuedapi.go` | DTOs and paths shared by control plane and gateway |
| `app/queue/handlers/gatewayapi/*` | switched to `pkg/queuedapi` |
| `internal/queued/config.go` | env config |
| `internal/queued/model/model.go` | `Queue`, `Settings`, `Outgoing`, `Delivery`, `DeadLetter`, `DeadReason`, `BackendStats` |
| `internal/queued/backend/backend.go` | the port |
| `internal/queued/backend/jetstream/*.go` | the JetStream driver |
| `internal/queued/signing/signing.go` | HMAC signature (shared format with the SDK) |
| `internal/queued/dispatch/outcome.go`, `delay.go`, `dispatcher.go` | outcome table, backoff, per-queue loop |
| `internal/queued/registry/registry.go`, `commands.go` | reconcile dispatchers and run commands |
| `internal/queued/publish/server.go` | publish endpoint |
| `internal/queued/controlplane/client.go` | desired/stats/done |
| `internal/queued/stats/stats.go`, `metrics.go` | counters, per-minute rates, Prometheus |
| `cmd/queued/main.go` | wiring and lifecycle |
| `Dockerfile`, `.github/workflows/release.yml` | `queued` image |

---

### Task 1: Shared DTOs, model, signing

- `pkg/queuedapi`: `Desired{Queues []Queue; Commands []Command}`, `Queue{ID, Name, Environment, Version, Settings, Paused, Target, PublisherHashes, SigningKey, PreviousKey}`, `Settings` (durations as strings, as plan 1), `Command{ID, QueueID, Kind, Argument}`, `StatsReport{Stats []Stats}` with `Stats` mirroring `app/queue/entities.Stats` JSON, paths `DesiredPath = "/queued/v1/desired"`, `StatsPath`, `DonePath(id)`.
- Switch `app/queue/handlers/gatewayapi` to these types; plan 1 integration tests keep passing unchanged.
- `internal/queued/model`: parsed settings (`time.Duration`s) with `FromAPI(queuedapi.Queue) (Queue, error)`; stream naming `StreamName(env, name)`, `DeadStream`, `DelayStream`, `Subject`, `DeadSubject`, `DelaySubject`.
- `internal/queued/signing`: `Sign(key string, t time.Time, body []byte) string`, `Verify(header string, keys []string, body []byte, now time.Time, maxAge time.Duration) error`.
- Tests: `Test_FromAPI_ParsesDurations`, `Test_FromAPI_RejectsBadDuration`, `Test_StreamNames`, `Test_Sign_VerifyRoundTrip`, `Test_Verify_RejectsTamperedExpiredAndUnknownKey`, `Test_Verify_AcceptsThePreviousKey`, and the existing gateway integration tests.
- Commit (with Task 2): storage/backend layer.

### Task 2: Backend port and JetStream driver

- Port (`internal/queued/backend`):

```go
type Backend interface {
	Ensure(ctx context.Context, q model.Queue) error
	Remove(ctx context.Context, q model.Queue) error
	Publish(ctx context.Context, q model.Queue, m model.Outgoing) (string, error)
	Fetch(ctx context.Context, q model.Queue, max int, wait time.Duration) ([]model.Delivery, error)
	MoveDue(ctx context.Context, q model.Queue, max int) (int, error)
	Ack(ctx context.Context, d model.Delivery) error
	Retry(ctx context.Context, d model.Delivery, after time.Duration) error
	Bury(ctx context.Context, q model.Queue, d model.Delivery, reason model.DeadReason) error
	Dead(ctx context.Context, q model.Queue, limit int) ([]model.DeadLetter, error)
	Replay(ctx context.Context, q model.Queue, id string) error
	ReplayAll(ctx context.Context, q model.Queue) (int, error)
	PurgeDead(ctx context.Context, q model.Queue) error
	Stats(ctx context.Context, q model.Queue) (model.BackendStats, error)
}
```

- `model.Outgoing{ID, Body, ContentType, IdempotencyKey, Due time.Time}`; `model.Delivery{ID, Queue, Body, ContentType, PublishedAt, Attempt int, handle}` where `handle` is opaque (`any`) for the driver; `model.DeadReason{Status int, Error string, Attempts int}`; `model.DeadLetter` = queuedapi dead letter fields; `model.BackendStats{Depth int64, InFlight int, OldestAge time.Duration, DeadLetters int64}`.
- JetStream driver: `jetstream.New(js jetstream.JetStream)`; `Ensure` creates/updates the three streams (`MaxAge` = retention, `MaxMsgSize`, `Duplicates` 2m on the main and delay streams, file storage) and durable pull consumers `dispatch` (main, `AckWait` = delivery timeout + 5s, `MaxAckPending` = max(in flight × 2, 10), `AckPolicy` explicit) and `mover` (delay stream). `Publish` sets `Nats-Msg-Id` from the idempotency key (or a new id), headers `Platform-Message-Id`, `Platform-Published-At`, `Content-Type`; a due time in the future goes to the delay stream with `Platform-Due`. `MoveDue` fetches up to `max` from `mover`, NAKs-with-delay the not-yet-due and republishes the due ones to the main subject with the same `Nats-Msg-Id`, then acks them. `Bury` publishes to the dead subject with headers `Platform-Last-Status`, `Platform-Last-Error`, `Platform-Attempts` and the original ones, then acks. `Dead` reads the newest `limit` dead letters by sequence (direct get); `Replay` republishes one dead letter by message id and deletes it; `ReplayAll` does it for all; `PurgeDead` purges the dead stream; `Stats` from stream/consumer info (`NumPending` + `NumAckPending` = depth, `NumAckPending` = in flight, oldest age from the first pending message's timestamp, dead count = dead stream messages). `Remove` deletes the three streams.
- Tests against an embedded `nats-server` with JetStream on a temp dir (`internal/queued/backend/jetstream/natstest_test.go` helper): `Test_Publish_FetchAck`, `Test_Publish_DeduplicatesTheIdempotencyKey`, `Test_Retry_RedeliversAfterTheDelayWithTheNextAttempt`, `Test_Bury_MovesToDeadLettersWithTheReason`, `Test_ReplayAndReplayAll_ReturnDeadLettersToTheQueue`, `Test_PurgeDead`, `Test_DelayedPublish_IsMovedWhenDue`, `Test_Stats_CountsDepthInFlightAndDead`, `Test_Unacked_IsRedeliveredAfterAckWait` (Review Focus 5), `Test_Ensure_UpdatesRetentionAndKeepsMessages`, `Test_Remove_DeletesTheStreams`.
- Commit: "Keep queue messages in JetStream behind a backend port" (storage/backend layer, with Task 1).

### Task 3: Dispatch rules and the per-queue dispatcher

- `dispatch.Outcome(status int, err error, header http.Header) (Verdict, time.Duration)` with `Verdict` = `Ack | Retry | Bury`; `dispatch.Delay(s model.Settings, attempt int, retryAfter time.Duration, jitter func() float64) time.Duration`.
- `dispatch.Dispatcher`: `New(backend, client *http.Client, q model.Queue, recorder Recorder) *Dispatcher`; `Run(ctx)`; `Update(q model.Queue)`; `Stop()`. Loop: when paused, sleep and fetch nothing; otherwise `MoveDue`, wait on `rate.Limiter` and a semaphore of `max_in_flight`, `Fetch`, deliver each in a goroutine: POST with signed headers and `context.WithTimeout(delivery_timeout)`, then Ack/Retry/Bury by the outcome; attempts ≥ `max_attempts` with a retry verdict → Bury. `Recorder` interface: `Delivered(queueID string, verdict Verdict, took time.Duration)`.
- Tests: `Test_Outcome_Table` (every row of the table), `Test_Delay_ExponentialFixedJitterAndRetryAfterCap`, dispatcher tests with the JetStream test server and an `httptest.Server` consumer: `Test_Dispatcher_DeliversSignedMessages`, `Test_Dispatcher_RetriesThenBuriesAfterMaxAttempts`, `Test_Dispatcher_PermanentFailureGoesStraightToDead`, `Test_Dispatcher_CutsAHangingConsumerAndRetries` (Review Focus 1), `Test_Dispatcher_HonoursRateAndInFlight`, `Test_Dispatcher_PausedDeliversNothing`.
- Commit (with Task 4): rules layer.

### Task 4: Registry and commands

- `registry.Registry`: `New(backend, factory DispatcherFactory)`; `Apply(ctx, queuedapi.Desired) []error` — ensures streams and starts dispatchers for new queues, updates changed ones (version **or** paused changed), stops dispatchers of queues missing from desired state (streams kept), runs commands: `replay`, `replay_all`, `purge_dead`, `delete` (stop + `Remove`), returns ids of done commands; `Lookup(name string, tokenHash string) (model.Queue, LookupResult)` for the publish server (`Found | Forbidden | NotFound`), `Keys(queueID) []string`, `Queues() []model.Queue`.
- `controlplane.Client`: `Desired(ctx)`, `Report(ctx, []queuedapi.Stats)`, `Done(ctx, id)`.
- Tests: `Test_Apply_StartsUpdatesAndStopsDispatchers`, `Test_Apply_ReconcilesOnPausedWithoutVersionChange`, `Test_Apply_MissingQueueKeepsItsStreams`, `Test_Apply_RunsCommandsAndReportsThemDone`, `Test_Apply_ReconfiguresInFlightWithoutLosingAcks` (Review Focus 2), `Test_Lookup_ByNameAndTokenHash`, client tests against `httptest`, `Test_Loop_SurvivesAnUnreachableControlPlane` (Review Focus 3).
- Commit: "Deliver queue messages under each queue's rate, concurrency and retry policy" (rules layer).

### Task 5: Publish server, stats, metrics, binary, image

- `publish.Server`: `POST /v1/queues/{name}/messages` per the constraints; `GET /healthz`.
- `stats.Collector`: implements `dispatch.Recorder`; per-minute delivered/failed over a 60s window; `Report(ctx, registry, backend) []queuedapi.Stats` (depth etc. from `Backend.Stats`, newest 50 dead letters with 4 KiB body preview); Prometheus gauges/counters per the spec on `QUEUED_METRICS_ADDR`.
- `cmd/queued/main.go`: config, NATS connect (retrying), loop: poll desired every `QUEUED_POLL` → `registry.Apply` → `Done` for finished commands; stats every `QUEUED_STATS_EVERY`; graceful shutdown stops dispatchers and drains in-flight deliveries.
- `Dockerfile`: build `/out/queued`, stage `queued` (alpine, non-root uid 10002, `EXPOSE 8080 9090`); `release.yml` builds and pushes `adrianozdp/platform-queued:<sha>`.
- Tests: `Test_Publish_AcceptsAKnownPublisher`, `Test_Publish_Rejects401403404413`, `Test_Publish_DeduplicatesTheIdempotencyKey`, `Test_Publish_PausedQueueAcceptsAndDeliversAfterResume` (Review Focus 4), `Test_Collector_PerMinuteRates`, and an end-to-end test in `internal/queued/e2e_test.go`: embedded NATS + fake control plane serving desired state + consumer `httptest.Server` → publish over HTTP, signed delivery verified, stats reported, replay command executed and confirmed.
- `README.md`: a "Queue gateway" section with the env vars.
- Commit: "Run queues from a gateway: publish endpoint, stats and the queued binary" (handlers/wiring layer). Then PR, merge after CI, release (image built; deployment comes with plan 3).

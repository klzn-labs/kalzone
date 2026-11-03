# Queues

## Queues

Teams create work queues from the API (`/api/v1/queues`): one consumer service per
queue and environment, a list of publisher services, and settings for rate limit,
in-flight deliveries, attempts, backoff, delivery timeout, retention and message size.
Each change is a numbered version in the audit log. A per-host gateway, `queued`,
reads the queues from `/queued/v1/desired` and runs them; see the
[managed queues design](superpowers/specs/2026-10-08-managed-queues-design.md).

Every deployed service receives `PLATFORM_QUEUE_URL`, `PLATFORM_QUEUE_TOKEN` (to
publish) and `PLATFORM_QUEUE_SIGNING_KEY` (to verify deliveries). The platform sets
them; the config API refuses those names.

## Queue gateway

`cmd/queued` runs the queues, built into `adrianozdp/platform-queued`. It takes
publishes on `POST /v1/queues/{name}/messages` (bearer `PLATFORM_QUEUE_TOKEN`,
optional `Idempotency-Key` and `Platform-Delay` in seconds), keeps messages in NATS
JetStream (streams `Q_`, `QD_` for delayed and `DLQ_` for dead letters per queue),
and delivers each with a signed POST to the consumer under the queue's rate limit,
in-flight limit and retry policy. It polls the control plane for queues and
commands and reports stats; Prometheus metrics are on the metrics address.

| Variable | Default | Purpose |
|---|---|---|
| `QUEUED_PLATFORM_URL` | | control plane base URL |
| `QUEUED_TOKEN` | | the control plane's `PLATFORM_QUEUES_TOKEN` |
| `QUEUED_NATS_URL` / `_USER` / `_PASSWORD` | | JetStream connection |
| `QUEUED_ADDR` | `:8080` | publish endpoint |
| `QUEUED_METRICS_ADDR` | `:9090` | `/metrics` |
| `QUEUED_POLL` | `5s` | desired state poll |
| `QUEUED_STATS_EVERY` | `15s` | stats report |

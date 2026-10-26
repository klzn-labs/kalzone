# Adaptive queue rates — design

Date: 2026-10-23
Status: agreed with the user in conversation on 2026-10-23

## 1. Why

Every queue drains at a fixed `rate_per_second`, applied in queued as a token bucket
(`internal/queued/dispatch/dispatcher.go`, `rate.NewLimiter(rate, 1)`). Most consumers write
to the one shared MySQL. A fixed rate is either too low, so work waits while the database
is idle, or too high, so a burst of messages competes with the apps' interactive queries.

gaardrail (`github.com/adrianozp/gaardrail`, the user's own PFC project) is a PID
back-pressure controller built for exactly this: it drives a `rate.Limiter` from a
measured database CPU and holds the CPU at a setpoint. The goal is to let a queue opt into
a rate that queued adjusts from the shared MySQL's CPU, using gaardrail's controller as a
library.

## 2. Decisions the user made

- This applies to **queues only**, never crons.
- gaardrail is the user's own project and can be changed as needed. Autopid is removed
  from it.
- gaardrail is embedded as a library in queued. It does not run as a separate service.

## 3. Goals and non-goals

Goals:

- A queue can be set to **adaptive**. Its rate then moves between a floor and its
  `rate_per_second`, driven by MySQL CPU against a platform setpoint.
- Without fresh data the behaviour is today's: the queue runs at its fixed rate.
- The current factor, measured CPU, setpoint and each queue's effective rate can be seen in
  metrics, in the API and MCP (`getQueueStats`), and on the queue page.

Non-goals (this round):

- A signal per schema or per database (perf_schema statement time). This is the next step
  once server CPU is proven.
- Controllers per queue, signals chosen per team, tuning of gains by teams, or autopid.
- Rate limits on publishing.

## 4. The model

### One governor in queued

- queued runs **one** control loop, the governor. All queues share one MySQL, so one loop
  per queue on the same signal would fight itself.
- Each period T, the governor:
  1. reads one PromQL value from Prometheus;
  2. runs it through gaardrail's measurement filter and its PI controller (feedforward
     off until the gain is identified);
  3. outputs a **factor** in `[0, 1]`.
- Every adaptive queue then gets `effective = min_rate + factor × (rate_per_second − min_rate)`
  and the dispatcher calls `limiter.SetLimit(effective)`.
- Fixed queues are never touched.
- The factor is the controller's output, so the controller's `Min`/`Max` are 0 and 1. A
  factor is less sensitive than raw messages per second to the queue mix changing. *(My
  call.)*

### Queue settings (versioned like every other setting, so prod changes go through a change request)

| Field | Meaning | Default |
|---|---|---|
| `adaptive` | Opt in to the governor | `false` |
| `min_rate_per_second` | Floor while adaptive; 0.1 ≤ min ≤ `rate_per_second` | 0.1 |

`rate_per_second` keeps its meaning as the fixed rate, and becomes the ceiling when the
queue is adaptive. The validation in `app/queue/entities/validate.go` gains the new rules.
The fields travel through `pkg/queuedapi.Settings` to `internal/queued/model.Settings`.

### Platform configuration (queued env, set in `kalzone-infra/stack/compose.platform.yml`)

| Variable | Default | Meaning |
|---|---|---|
| `QUEUED_ADAPTIVE_PROMETHEUS_URL` | empty: governor off, factor fixed at 1 | e.g. `http://prometheus:9090` |
| `QUEUED_ADAPTIVE_QUERY` | `sum(rate(container_cpu_usage_seconds_total{container_label_com_docker_compose_service="mysql"}[1m]))*100` | process variable, in % of one core |
| `QUEUED_ADAPTIVE_SETPOINT` | chosen by the builder | target, same unit |
| `QUEUED_ADAPTIVE_PERIOD` | `15s` | control period T, equal to the cAdvisor scrape interval |
| `QUEUED_ADAPTIVE_KP` / `_KI` | conservative start gains | PI gains on the factor |
| `QUEUED_ADAPTIVE_STALE_AFTER` | `3` periods | how many failed or old reads until fail-safe |

Things for the builder to check:

- The `mysql` container has no CPU limit, so the value is a percentage of one core and can
  pass 100 on a multi-core host. Check the host's core count and the current baseline
  CPU in Prometheus before choosing the setpoint. Write the choice and the reason in the
  PR description.
- The window must cover at least 3 scrapes (`[1m]` at the 15s scrape).

### Fail-safe

- A read counts as failed when Prometheus is unreachable, returns an empty result or NaN,
  or the sample's timestamp is older than 2×T.
- After `STALE_AFTER` failed reads in a row, the governor resets the controller and moves
  the factor to 1 over a few periods. That is today's fixed-rate behaviour, so losing the
  signal is never worse than not having the feature.
- The first good read starts the controller again from a clean state.
- The governor logs and counts every move into and out of fail-safe. *(My call.)*

### Tuning

- gaardrail's documented gains are in messages per second for a different plant, and are
  not valid for a factor. The builder starts with a slow, conservative PI.
- In dev, the builder runs a step on the factor against a test queue and identifies K and
  τ (FOPDT, two points, as in the PFC). Then they set the gains with SIMC or AMIGO by hand
  and record the numbers in the PR.
- Gains are env values, so retuning needs no release.

## 5. Visibility

- **queued `/metrics`** (already scraped by job `queued`):
  - `queued_adaptive_factor`, `queued_adaptive_measured`, `queued_adaptive_setpoint`
  - `queued_adaptive_term{term="p|i"}`
  - `queued_adaptive_failsafe` (0/1)
  - `queued_queue_effective_rate{queue,environment}`
- **Stats report:** `queuedapi.Stats` gains `effective_rate_per_second`, and the report
  gains the governor state. `getQueueStats` (API and MCP) returns both.
- **Queue page:** one line, e.g. "Adaptive: 7.3/s of 10/s (DB CPU 54%, target 60%)", or
  "Fixed: 10/s". The edit form gets an Adaptive toggle and a Minimum rate field. A mockup
  comes first, per the standing rule for visual changes.
- **Grafana:** the platform dashboard gets a panel with factor and DB CPU against the
  setpoint.

## 6. gaardrail changes (separate repo, done first)

- Remove autopid.
- Move the PI/PIFF controller (and Smith and step, where they fit) plus the filters into
  an importable `pkg/`.
  - Constructors take explicit params and return errors.
  - No global metrics: `Compute` returns its terms.
  - Safe for concurrent use.
- Fix the bugs already found:
  - On the first tick, dt is measured from zero time, which saturates the integral.
  - `SetSetpoint` runs without the lock.
  - The orchestrator panics on shutdown.
  - The `-race` test fails.
- Tag a release that queued's `go.mod` can require.

## 7. Order of delivery

1. **gaardrail:** library, autopid removed, bug fixes, release tag.
2. **Storage and rules:** queue settings `adaptive` and `min_rate_per_second`, validation,
   and the queuedapi wire fields.
3. **queued:** the governor (`internal/queued/governor`) using the gaardrail library, the
   dispatcher applying the factor, fail-safe, metrics, stats fields, env config, and tests
   with a fake Prometheus and a fake clock.
4. **UI:** a mockup first, then the queue page line and the form fields.
5. **Infra:** the env vars in `compose.platform.yml` and the Grafana panel.
6. **Tuning:** the step test in dev, the gains, then enable it on one prod queue (Plateful
   `place-import`).

Steps 2–3 start after the cron executor lands, because both change `internal/queued/app.go`
and the runner.

## 8. Verification

- **Unit tests:**
  - The governor goes into fail-safe after N bad reads and comes back on a good one.
  - The effective rate formula, including that `min == max` gives a constant rate.
  - Fixed queues are untouched.
  - The factor stays within `[0, 1]`.
  - A change of queue version keeps the adaptive state.
- **Closed loop:** a fake Prometheus fed by a simulated first-order plant reaches the
  setpoint without sustained oscillation.
- **Integration:** the settings round-trip through API and MCP (`updateQueue`,
  `getQueueStats`); prod updates still become a change request.
- **Live, in dev:**
  1. Make a test queue adaptive and flood it.
  2. Watch MySQL CPU settle near the setpoint in Grafana.
  3. Stop Prometheus and see the fail-safe return the queue to its fixed rate.

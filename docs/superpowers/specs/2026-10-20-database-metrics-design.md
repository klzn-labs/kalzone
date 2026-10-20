# Database metrics on the service dashboard

Status: draft, 2026-10-20. Reviewed by a builder check.

## Why

When a service's database misbehaves, its Grafana dashboard says nothing about it. The dashboard should answer two questions at a glance: is the database under strain, and is this service the cause? It should also name the queries to fix.

Mockup the user approved: https://claude.ai/artifact/Aupifck3sfrDmaSNJE4YYs

## Goals

A new **Database** row on the per-service dashboard (`app/observe/entities/dashboard.go`), between the service panels and Logs. It follows the existing `$env` (dev/prod) switch and the time picker.

| Panel | Shows | Scope |
|---|---|---|
| Connections | open connections of the service's MySQL user, plus its `max_connections` as a dashed limit line | this service |
| Query time | seconds per second MySQL spends on this schema's statements | this service |
| MySQL CPU | CPU of the shared MySQL container, titled or tagged "shared by all services" | whole server |
| Average query time | average statement latency for this schema | this service |
| Slowest queries | table: normalized query text, calls, average time, total time, rows examined per call; slowest average first; over the selected range | this service |

User decisions:

- Both CPU views: the shared container CPU and this service's query time. MySQL cannot measure CPU per schema.
- Slow queries are shown as which queries (a table of query shapes), not a count over a threshold.
- Average time per selected range is enough. Worst single run and p95 are not needed.
- `performance_schema` is turned on with its default instruments, not a trimmed set, and MySQL gets more resources to pay for it (see below).
- Any signed-in user may see every service's dashboard, query text included, for now. Per-team Grafana access is a separate, later project.

## Non-goals

- No worst-run or p95 columns, and no Grafana MySQL data source querying `performance_schema` live.
- No alerts.
- No database panels in the Kalzone web UI.
- No change to the global `mysql.json` dashboard.
- No per-team access in Grafana (deferred; recorded as pending work).

## Anti-goals

- Schema, user or query text of one service must not become publicly reachable. The control plane's `/metrics` is public today (`https://platform.kalz.one/metrics` answers 200), so anything about attachments must not go there.
- A service's panels must never show another service's numbers, except the MySQL CPU panel, which says it is shared.
- The exporter must not eat the VPS: it has `mem_limit: 64m` and its series count must stay bounded.

## Where the data comes from

**kalzone-infra, mysqld-exporter (v0.17.2).** It runs default collectors only today. Enable:

- `--collect.info_schema.processlist` for connections per user (`mysql_info_schema_processlist_processes_by_user{mysql_user}`).
- `--collect.perf_schema.eventsstatements` for per-digest statement stats with `schema`, `digest` and `digest_text` labels: count, seconds, rows examined. Query time, average time and the table all come from these.

Raise its digest limit and digest text length so every service with traffic appears and the query text is readable. Exact flag names and values are the builder's to verify against v0.17.2. The exporter user already has `PROCESS` and `SELECT ON *.*`; check it reads `performance_schema` under `partial_revokes=ON`.

**kalzone-infra, MySQL.** `stack/compose.data.yml` runs it with `--performance-schema=OFF`, `--innodb-buffer-pool-size=128M`, `--max-connections=50` and `mem_limit: 512m`. On 2026-10-20 it used 278 MiB, and the 6 GB host had 3.9 GB available. Changes:

- Remove `--performance-schema=OFF` and keep the default instruments (user decision).
- Rescale (user decision; the numbers are my call): `mem_limit` 1g, `innodb-buffer-pool-size` 384M, `max-connections` 150. Today two adopted databases may each take 30 connections against a server limit of 50, so the server limit is already too low.
- Also raise the exporter's `mem_limit` if the digest series need it.
- Dev and prod share this one MySQL server, so measure real memory on a local stack before shipping. The restart takes every service's database down in both envs, so it goes in a quiet window.

**cadvisor** already gives the MySQL container's CPU (`container_cpu_usage_seconds_total`); the builder finds its container name.

**Which schema and user belong to the service in each env.** A service has at most one active attachment per env (`app/database/entities`: `Database.Schema`, `Database.Username`, `Database.MaxConnections`; `Attachment.ServiceID`, `Attachment.Environment`). Schemas differ per env (`plateful` in prod, `plateful_dev` in dev), so the panels need a per-env mapping.

My call: Kalzone publishes one info series per active attachment, `kalzone_database_attachment{service, environment, schema, mysql_user} = max_connections`, through the internal pushgateway that deployctl already uses (scraped with `honor_labels`). It is written when a database is attached, detached, adopted or deleted, and on control-plane start, alongside the dashboard sync. Panels join on it with `$env`, so dev/prod switching keeps working and the limit line is the series value. The builder checks that the control plane reaches the pushgateway; if it cannot, an internal-only scrape target is acceptable. The public `/metrics` is not.

## Dashboard behaviour

- The Database row appears only when the service has an active attachment in at least one env (my call). Dashboards are regenerated on `database.attached`, `database.detached`, `database.adopted` and `database.deleted` as well as the existing service events, and on start.
- In an env with no attachment, the row's panels show "No data".
- A database attached to several services (prod `plateful`: api, worker, importer) cannot be split per service: the row is titled "Database (shared with importer, worker)" and its numbers cover the whole database (my call).
- Panel ids continue the existing constants (`PanelRequests` …). Existing panels and Logs move down but keep their ids.

## Left to the builder

- PromQL details, rate windows and table transformations, as long as the table follows the time range (for example `increase` over `$__range`).
- Grid sizes, colours and units, following the mockup.
- Tests: dashboard JSON for a service with and without an attachment, the info-series writer, and the event triggers.
- Delivery: kalzone-infra (exporter flags, any scrape change) ships before kalzone.

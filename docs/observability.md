# Diagnostics and telemetry

## Diagnostics

- `GET /api/v1/runtime/:service/:env` (`getRuntime`, scope `read`) shows the live
  container: `state`, `health` and the last health check outputs, `restarts`,
  `exit_code`, `oom_killed`, `image`, `started_at`, and `usage` (CPU millicores and
  memory MB next to the container's limits, from Prometheus; left out when Grafana or
  Prometheus cannot answer). The agent reports every 15s (`AGENT_STATUS_EVERY`);
  `stale: true` means it has not reported for over a minute.
- `GET /api/v1/logs/:service/:env` (`listLogs`) reads both colours' container logs
  from Loki, oldest first: `since` (default `15m`, at most `24h`), `limit` (1-1000,
  default 100, the newest lines in the window), `contains`, `colour`. It needs a
  member of the owning team or an admin, and on a token both `read` and `logs`.
- A failed deployment carries `failure`: `class` (`timeout`, `oom_killed`,
  `unhealthy`, `crashed`, `start_failed`, `smoke_failed`, `gate_failed`,
  `platform_error`), a `hint`, the failed candidate `container`, its `exit_code`,
  `oom_killed`, `health_log` and its last 50 log lines in `logs`. deployctl exits 10
  (start), 11 (smoke), 12 (traffic gate) or 13 (anything else), and the agent
  inspects the candidate it left stopped.
- Both reads are MCP tools too; `listLogs` shows only for tokens with `logs`.
- Grafana's datasource uids are `PLATFORM_GRAFANA_PROMETHEUS_UID` (`prometheus`) and
  `PLATFORM_GRAFANA_LOKI_UID` (`loki`).
- `/telemetry/:team/prometheus/*` and `/telemetry/:team/loki/*` are the datasources of
  a team's Grafana org. They take `Authorization: Bearer <token>`, an HMAC of the team
  slug under `PLATFORM_TELEMETRY_KEY`. Prometheus answers only reads, and every
  selector gets a matcher for the team's deployments (`service` on Traefik series,
  `name` on containers, `mysql_user` and `schema` on MySQL ones); a selector without
  a known metric name matches nothing. Loki answers only reads, with `X-Scope-OrgID`
  set to the team's tenants (`<service>-<env>`). `PLATFORM_TELEMETRY_ALL_TOKEN` opens
  the `all` scope, which reads every Loki tenant plus `platform` and `fake`, and no
  Prometheus.
- Every team has a Grafana org `team-<slug>`. Its members are Viewers and platform
  admins are Admins in every org; it holds the team's service dashboards and
  `prometheus`/`loki` datasources pointing at the team's `/telemetry`. The demo team's
  services live in one `demo` org on Grafana's TestData, open to the guest and to people
  on no team. The main org keeps every dashboard for admins. The dashboards worker
  writes all of it at start, every 5 minutes, and on team, membership and admin
  changes, and it deletes `team-*` orgs whose team is gone. A proxied non-admin sends
  no role, so Grafana's `None` default keeps them out of the main org.

# Database Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Database row on every per-service Grafana dashboard whose service has a database: connections against the limit, this service's query time, shared MySQL CPU, average query time and a slowest-queries table.

**Architecture:** kalzone-infra turns on `performance_schema`, rescales MySQL and enables two exporter collectors. Kalzone publishes one info series per active attachment to the internal pushgateway (`kalzone_database_attachment{service,environment,schema,mysql_user} = max_connections`). The generated dashboards join MySQL metrics on it through `$env`. Every service or database event re-syncs all dashboards and the info series.

**Tech Stack:** Go, Uber fx, Grafana dashboard JSON, Prometheus/PromQL, pushgateway text format, mysqld-exporter v0.17.2, MySQL 8.0, docker compose.

**Spec:** `docs/superpowers/specs/2026-10-20-database-metrics-design.md`

## Global Constraints

- No comments in any file, including YAML, compose and tests (global CLAUDE.md). Function bodies ≤ 20 lines and at most 2 nesting levels.
- Nothing about attachments goes on the control plane's public `/metrics`.
- MySQL: `mem_limit: 1g`, `--innodb-buffer-pool-size=384M`, `--max-connections=150`, and no `--performance-schema=OFF`.
- Info series name and labels exactly: `kalzone_database_attachment{service="<slug>",environment="<env>",schema="<schema>",mysql_user="<user>"} <max_connections>`, pushgateway job `kalzone_databases`.
- The MySQL CPU panel says it is shared by all services.
- The Database row appears only when the service has an active attachment in at least one env.
- Commits: one per layer at the end (`layered-commits` skill). Infra ships before kalzone. No Co-Authored-By trailer.

## Review Focus

1. **A database shared by several services** (prod `plateful` is attached to api, worker and importer): the row's numbers cover the whole database, so the row title must say "Database (shared with importer, worker)". Pinned in Task 3 and Task 4.
2. **The service has a database in prod only:** the row exists. Picking dev shows "No data", never another service's numbers. Each query is filtered by `service` and `environment`. Pinned in Task 3.
3. **The last attachment goes away** (detach, delete or service deletion): the info series must disappear, not linger in pushgateway. An empty publish DELETEs the group. Pinned in Task 2 and Task 4.
4. **Pushgateway down, or Grafana not configured:** the other half still runs, and errors are logged without blocking start. Pinned in Task 4.
5. **Query text with quotes or backslashes in labels:** pushgateway lines quote label values with `strconv.Quote`. Pinned in Task 2.

---

### Task 1: MySQL statement stats and exporter collectors (kalzone-infra)

**Files:**
- Modify: `kalzone-infra/stack/compose.data.yml` (services `mysql`, `mysqld-exporter`)

Work on branch `database-metrics` in kalzone-infra.

- [ ] **Step 1: Check the exporter flags against the pinned image**

Run: `docker run --rm prom/mysqld-exporter:v0.17.2 --help 2>&1 | grep -E "processlist|eventsstatements"`
Expected: flags `--collect.info_schema.processlist`, `--collect.perf_schema.eventsstatements`, `--collect.perf_schema.eventsstatements.limit`, `--collect.perf_schema.eventsstatements.digest_text_limit`. Use the spellings it prints.

- [ ] **Step 2: Edit `mysql`**

Set the command to `--innodb-buffer-pool-size=384M`, `--max-connections=150` (drop `--performance-schema=OFF`) and `mem_limit: 1g`.

- [ ] **Step 3: Edit `mysqld-exporter`**

Add `--collect.info_schema.processlist`, `--collect.perf_schema.eventsstatements`, `--collect.perf_schema.eventsstatements.limit=1000` and `--collect.perf_schema.eventsstatements.digest_text_limit=400`, and set `mem_limit: 128m`.

- [ ] **Step 4: Measure locally**

Run `docker compose -f stack/compose.data.yml up -d mysql mysqld-exporter` with a throwaway `.env` (passwords only), create the exporter user as `stack/scripts/up.sh ensure_exporter_user` does, and run a few queries in a scratch schema. Then:
- `curl -s localhost:9104/metrics | grep -E "^mysql_info_schema_processlist_processes_by_user|^mysql_perf_schema_events_statements_(total|seconds_total|rows_examined_total)" | head` shows series with labels `mysql_user`, and `schema`, `digest`, `digest_text`.
- `docker stats --no-stream` shows MySQL well under 1g and the exporter under 128m.

Record the exact label names. Task 3 uses `mysql_user` and `schema`; if the exporter's names differ, change the Global Constraints label names and Task 3 to match.

- [ ] **Step 5: Run the infra checks**

Run: `make test` (or whatever the infra repo's CI runs; `test/e2e.sh` brings up the stack)
Expected: PASS

- [ ] **Step 6: Commit** `Turn on MySQL statement stats and collect them per schema` (in kalzone-infra).

---

### Task 2: Pushgateway publisher for attachment series

**Files:**
- Create: `app/observe/repositories/pushgateway/pushgateway.go`
- Test: `app/observe/repositories/pushgateway/pushgateway_test.go`
- Modify: `app/observe/entities/dashboard.go` (types only, see Produces)
- Modify: `pkg/config/config.go` (add `Metrics`)

**Interfaces:**
- Produces, in `app/observe/entities`:
  - `type Database struct { Environment, Schema, User string; MaxConnections int; SharedWith []string }`
  - `Service` gains `ID string` and `Databases []Database`.
- Produces, in `pkg/config`: `Config.Metrics Metrics` with `type Metrics struct { PushgatewayURL string \`mapstructure:"pushgateway_url" default:"http://pushgateway:9091"\` }`.
- Produces, in `pushgateway`:
  - `func NewClient(cfg config.Config) Client`
  - `func (c Client) Enabled() bool` (URL non-empty)
  - `func (c Client) PublishAttachments(ctx context.Context, services []entities.Service) error`

- [ ] **Step 1: Write the failing tests** with an `httptest.Server` recording method, path and body:

```go
func Test_PublishAttachments_PutsOneSeriesPerAttachment(t *testing.T) {
	srv, got := recorder(t)
	c := pushgateway.NewClient(config.Config{Metrics: config.Metrics{PushgatewayURL: srv.URL}})
	err := c.PublishAttachments(context.Background(), []entities.Service{{Slug: "api", Databases: []entities.Database{
		{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30},
		{Environment: "dev", Schema: "plateful_dev", User: "kz_plateful_dev", MaxConnections: 10},
	}}, {Slug: "web"}})
	require.NoError(t, err)
	require.Equal(t, "PUT /metrics/job/kalzone_databases", got.request)
	require.Contains(t, got.body, `kalzone_database_attachment{service="api",environment="prod",schema="plateful",mysql_user="kz_plateful"} 30`+"\n")
	require.Contains(t, got.body, `kalzone_database_attachment{service="api",environment="dev",schema="plateful_dev",mysql_user="kz_plateful_dev"} 10`+"\n")
	require.NotContains(t, got.body, `service="web"`)
}

func Test_PublishAttachments_DeletesTheGroupWhenNothingIsAttached(t *testing.T) {
	srv, got := recorder(t)
	err := pushgateway.NewClient(config.Config{Metrics: config.Metrics{PushgatewayURL: srv.URL}}).PublishAttachments(context.Background(), []entities.Service{{Slug: "web"}})
	require.NoError(t, err)
	require.Equal(t, "DELETE /metrics/job/kalzone_databases", got.request)
}

func Test_PublishAttachments_QuotesLabelValues(t *testing.T) { /* Schema `a"b\c` appears as schema="a\"b\\c" */ }
func Test_PublishAttachments_FailsOnAnErrorStatus(t *testing.T) { /* server answers 500 → error */ }
```

- [ ] **Step 2: Run** `go test ./app/observe/repositories/pushgateway/...`. Expected: FAIL (package does not exist).

- [ ] **Step 3: Implement.** Use `http.Client{Timeout: 10 * time.Second}` and a text body with one line per attachment, labels quoted with `strconv.Quote`, `Content-Type: text/plain; version=0.0.4`. Treat any non-2xx as an error. Follow `kalzone-infra/deployctl/internal/pushgw/pushgw.go`.

- [ ] **Step 4: Run** `go test ./app/observe/... ./pkg/config/...`. Expected: PASS.

---

### Task 3: Database row in the generated dashboard

**Files:**
- Modify: `app/observe/entities/dashboard.go`
- Create: `app/observe/entities/database_panels.go` (keeps `dashboard.go` small)
- Test: `app/observe/entities/dashboard_test.go`

**Interfaces:**
- Consumes: `entities.Service.Databases`, `entities.Database` (Task 2).
- Produces: panel ids `PanelDatabaseRow = 8`, `PanelConnections = 9`, `PanelQueryTime = 10`, `PanelMySQLCPU = 11`, `PanelQueryLatency = 12`, `PanelSlowQueries = 13`. `func DatabaseRowTitle(dbs []Database) string`.

Layout (24-column grid): the row header is at y=16 (h=1). Connections, Query time and MySQL CPU are at y=17 (w=8, h=8). Average query time is at y=25 (w=8), with Slowest queries at x=8 (w=16), both h=10. Logs moves to y=35. Without databases, Logs stays at y=16 and no database panel is emitted.

PromQL (`A` = the info filter `kalzone_database_attachment{service="<slug>",environment="$env"}`):

| Panel | Unit | Targets |
|---|---|---|
| Connections | none | `sum(mysql_info_schema_processlist_processes_by_user and on(mysql_user) A)` legend "open"; `max(A)` legend "limit (max_connections)", red dashed override |
| Query time | s | `sum(rate(mysql_perf_schema_events_statements_seconds_total[$__rate_interval]) and on(schema) A)` |
| MySQL CPU, shared by all services | percentunit | `sum(rate(container_cpu_usage_seconds_total{name="plateful-mysql-1"}[$__rate_interval]))`; panel `description`: "Shared by every service on this MySQL server" |
| Average query time | s | `sum(rate(..._seconds_total[$__rate_interval]) and on(schema) A) / sum(rate(mysql_perf_schema_events_statements_total[$__rate_interval]) and on(schema) A)` |
| Slowest queries | table | three instant, table-format targets `sum by (digest_text) (increase(<metric>[$__range]) and on(schema) A)` for `_total` (Calls), `_seconds_total` (Total time), `_rows_examined_total`; transformations `merge`, `calculateField` binary Total time / Calls → "Avg time" and rows / Calls → "Rows examined / call", `organize` to Query, Calls, Avg time, Total time, Rows examined / call, `sortBy` Avg time desc |

Row title: "Database", or "Database (shared with x, y)", using the sorted union of `SharedWith` across the service's databases.

- [ ] **Step 1: Write the failing tests**

```go
func Test_Dashboard_HasNoDatabaseRowWithoutAttachments(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "web", Name: "Web"})
	require.NotContains(t, body, "kalzone_database_attachment")
	require.Contains(t, body, `"gridPos":{"h":12,"w":24,"x":0,"y":16}`)
}

func Test_Dashboard_AddsTheDatabaseRowFilteredByServiceAndEnv(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "api", Name: "API", Databases: []entities.Database{{Environment: "prod", Schema: "plateful", User: "kz_plateful", MaxConnections: 30}}})
	require.Contains(t, body, `"type":"row"`)
	require.Contains(t, body, `"title":"Database"`)
	require.Contains(t, body, `kalzone_database_attachment{service=\"api\",environment=\"$env\"}`)
	require.Contains(t, body, `and on(mysql_user)`)
	require.Contains(t, body, `and on(schema)`)
	require.Contains(t, body, `container_cpu_usage_seconds_total{name=\"plateful-mysql-1\"}`)
	require.Contains(t, body, "Shared by every service on this MySQL server")
	require.Contains(t, body, `increase(mysql_perf_schema_events_statements_seconds_total[$__range])`)
	require.Contains(t, body, `"gridPos":{"h":12,"w":24,"x":0,"y":35}`)
}

func Test_DatabaseRowTitle_NamesTheOtherServices(t *testing.T) {
	require.Equal(t, "Database", entities.DatabaseRowTitle([]entities.Database{{}}))
	require.Equal(t, "Database (shared with importer, worker)", entities.DatabaseRowTitle([]entities.Database{
		{Environment: "prod", SharedWith: []string{"worker", "importer"}}, {Environment: "dev", SharedWith: []string{"worker"}},
	}))
}
```

Keep the existing `Test_Dashboard_QueriesTheServicesWorkloadsPerEnvironment` passing.

- [ ] **Step 2: Run** `go test ./app/observe/entities/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** Reuse `timeseries()` for the four graphs. Add a `row()` builder and a `slowQueries()` table builder. Thread the y offset of Logs.
- [ ] **Step 4: Run** `go test ./app/observe/entities/...`. Expected: PASS.

---

### Task 4: Sync usecase for dashboards and attachment series

**Files:**
- Modify: `app/observe/usecases/dashboards/dashboards.go`
- Create: `app/observe/usecases/dashboards/sync.go`
- Test: `app/observe/usecases/dashboards/dashboards_test.go`, `app/observe/usecases/dashboards/sync_test.go`
- Modify: `app/observe/repositories/catalog/catalog.go` (new adapter `Services`)
- Test: `app/observe/repositories/catalog/catalog_test.go`

**Interfaces:**
- Consumes: `pushgateway.Client.PublishAttachments` (Task 2), `entities.Dashboard` (Task 3).
- Produces, in `dashboards`:
  - `type Catalog interface { Services(ctx context.Context) ([]entities.Service, error) }`
  - `type Publisher interface { Enabled() bool; PublishAttachments(ctx context.Context, services []entities.Service) error }`
  - `func NewDashboardsUsecase(grafana Grafana, catalog Catalog, publisher Publisher) DashboardsUsecase`
  - `func (u DashboardsUsecase) Sync(ctx context.Context) error`
  - `Handle` reacts to `service.created`, `service.updated`, `database.attached`, `database.detached`, `database.adopted`, `database.deleted` and `database.released` by calling `Sync`.
  - `func WithSharing(services []entities.Service) []entities.Service` fills each `Database.SharedWith` with the other services' slugs that have the same env and schema.
- Produces, in `observe/repositories/catalog`:
  - `type ServiceLister interface { List(ctx, catalog.ServiceFilter) ([]catalog.Service, error) }`
  - `type Attachments interface { AttachedTo(ctx context.Context, serviceID string) ([]database.Database, error) }`
  - `func NewServices(l ServiceLister, a Attachments) Services`
  - `func (s Services) Services(ctx context.Context) ([]entities.Service, error)` maps `Schema`, `Username`→`User`, `MaxConnections` and `Environment`.

`Sync` loads the services once, applies `WithSharing`, then runs `Write` (when Grafana is enabled) and `PublishAttachments` (when the publisher is enabled) independently. It returns the joined errors (`errors.Join`). `Write` stays as is.

- [ ] **Step 1: Write the failing tests** using fakes like the existing `grafana` fake:
  - `Test_Sync_WritesEveryDashboardAndPublishesTheAttachments`: a catalog with `api` (prod `plateful`) and `web` (none) leads to upserted uids `platform-api` and `platform-web`, and the publisher receives both services.
  - `Test_Sync_PublishesEvenWhenGrafanaIsOff` and `Test_Sync_WritesDashboardsWhenThePublisherFails`: the publisher returns an error, dashboards are still written, and `Sync` returns that error.
  - `Test_Handle_SyncsOnDatabaseEvents`: each of the seven actions triggers one `Services` call; `queue.created` triggers none.
  - `Test_WithSharing_NamesServicesOnTheSameSchema`: api, worker and importer on prod `plateful` give api `SharedWith` = `["importer","worker"]` (sorted). A service on dev `plateful_dev` alone gets an empty `SharedWith`.
  - `Test_Services_MapsAttachmentsPerService` in the catalog adapter, with fake lister and attachments.
- [ ] **Step 2: Run** `go test ./app/observe/...`. Expected: FAIL.
- [ ] **Step 3: Implement** as specified in Interfaces.
- [ ] **Step 4: Run** `go test ./app/observe/...`. Expected: PASS.

---

### Task 5: Wiring, start-up sync and integration test

**Files:**
- Modify: `app/observe/handlers/sync/sync.go` (`OnStart` calls `Sync`; drop `Services` and `Writer`, take `type Syncer interface { Sync(ctx context.Context) error }`)
- Modify: `cmd/controlplane/modules/observe.go` (provide `pushgateway.NewClient`, `catalog.NewServices` with `services.ServicesUsecase` and `databases.DatabasesUsecase`, and the `Catalog`/`Publisher`/`Syncer` bindings)
- Create: `internal/fakepushgateway/fakepushgateway.go` (records the last PUT body per job and DELETEs; `Body(job string) string`)
- Modify: `internal/fakegrafana/fakegrafana.go` (keep each dashboard's JSON; add `DashboardJSON(uid string) string`)
- Modify: `internal/integration/suite.go` (start the fake and set `cfg.Metrics.PushgatewayURL`)
- Test: `internal/integration/tests/database_metrics_test.go`

- [ ] **Step 1: Write the failing integration test**

```go
func Test_AttachedDatabaseShowsOnTheServiceDashboard(t *testing.T) {
	s, p, _ := setupDatabases(t)
	created := createDatabase(t, p.bob, "dev")
	require.Equal(t, http.StatusCreated, created.Status, created.Body)
	attachShop(t, p.bob)

	require.Eventually(t, func() bool {
		return strings.Contains(s.Pushgateway.Body("kalzone_databases"), `service="places-api",environment="dev",schema="kzt_shop_dev",mysql_user="kz_kzt_shop_dev"`)
	}, 5*time.Second, 50*time.Millisecond)
	require.Contains(t, s.Grafana.DashboardJSON("platform-places-api"), `"title":"Database"`)

	detachShop(t, p.bob)
	require.Eventually(t, func() bool { return !strings.Contains(s.Pushgateway.Body("kalzone_databases"), "places-api") }, 5*time.Second, 50*time.Millisecond)
	require.NotContains(t, s.Grafana.DashboardJSON("platform-places-api"), `"type":"row"`)
}
```

- [ ] **Step 2: Run** `go test ./internal/integration/tests/ -run Test_AttachedDatabaseShowsOnTheServiceDashboard`. Expected: FAIL.
- [ ] **Step 3: Implement** the wiring and fakes listed above.
- [ ] **Step 4: Run** `make test` (the full suite, `golangci-lint` included). Expected: PASS.
- [ ] **Step 5: Shape the history** with the `layered-commits` skill on top of the spec commit: storage (Task 2 + the catalog adapter), rules (Task 3 + the usecase), handlers and wiring (Task 5). This plan file goes in with the spec commit.

---

### Task 6: Ship

- [ ] **Step 1:** Open, then rebase-merge, the kalzone-infra PR (memory: rebase-only). Apply through Terraform-in-Docker (mount the whole repo; see the plateful-vps memory). MySQL restarts here, so do it in a quiet window and tell the user first. Afterwards confirm in Grafana Explore that `mysql_perf_schema_events_statements_total` has series, and check `docker stats` for MySQL.
- [ ] **Step 2:** Open, then rebase-merge, the kalzone PR and release the control plane the usual way. On start the sync publishes the info series. Check that `kalzone_database_attachment` in Prometheus lists api, worker, importer and legendsgrid, and open `platform-api` in Grafana to see the Database row "Database (shared with importer, worker)".

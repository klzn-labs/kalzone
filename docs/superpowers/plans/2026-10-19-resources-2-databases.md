# Resources 2 — Managed Databases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teams create a MySQL database on the shared server, attach it to services, and the platform injects scoped credentials at deploy, from the UI, the API and MCP.

**Architecture:** A new `app/database` domain: a `databases` table plus `database_attachments`, a MySQL **provisioner** adapter that talks to the server with a dedicated admin account, and a `databases` usecase that follows the queue domain's shapes (authz gate, change requests for protected environments, audit events). The deploy domain gets a second access port, next to `QueueAccess`, that merges `PLATFORM_DB_*` into the resolved config at claim. Change requests learn to point at a Resource instead of a service.

**Tech Stack:** Go 1.26, gin, gorm/MySQL, `database/sql` + `go-sql-driver/mysql` for the provisioner, Uber fx, htmx templates, testify.

**Spec:** `docs/superpowers/specs/2026-10-19-resources-design.md` (delivery step 2). Depends on plan `2026-10-19-resources-1-section.md` being merged.

## Global Constraints

- No comments in any file (Go, SQL, HTML, tests). Functions ≤ 20 lines, ≤ 2 levels of nesting.
- Database name: `^[a-z][a-z0-9-]{1,19}$`. Schema: name with `-`→`_`, plus `_dev` in `dev`; in a protected environment the bare name. Other unprotected environments: `_<env>`.
- MySQL user: `kz_<schema>`, at most 32 characters (refuse creation otherwise). Password: 32 random bytes, hex. Grant: `ALL PRIVILEGES ON \`<schema>\`.*` only. `MAX_USER_CONNECTIONS` = the database's `max_connections`, default `10`, allowed `1..30`.
- Reserved schemas: `mysql`, `sys`, `information_schema`, `performance_schema`, `platform`. Creating a database whose schema already exists in MySQL is refused (adoption is plan 3).
- Injected: env `PLATFORM_DB_HOST`=`mysql`, `PLATFORM_DB_PORT`=`3306`, `PLATFORM_DB_NAME`; secrets `PLATFORM_DB_USER`, `PLATFORM_DB_PASSWORD`, `PLATFORM_DB_DSN`=`<user>:<pass>@tcp(mysql:3306)/<schema>?parseTime=true`. All six names join `reserved` in `app/envconfig/entities/validate.go`. Platform values win on merge.
- At most one database attached per service per environment (an active attachment, `detached_at IS NULL`).
- A password never appears in an API response, page, log line, audit detail or change request payload.
- Delete refused while any attachment is active, or detached but still running (see Task 4). Drops schema, then user, then the row.
- Attach/detach take effect on the next deploy; the API message and the UI say so. No automatic redeploy.
- Authorization: new action `EditDatabase`: member in unprotected envs, maintainer in protected ones (`roleForEnvironment`), token scope `services`, protectable (change request in prod). Attach additionally needs `EditSecrets` on the target service's team and environment.
- Visual changes: mockups as an artifact first, user picks.
- One commit per layer at the end via layered-commits: storage → provisioner → rules → handlers+wiring+integration → infra (separate repo).

## Review Focus

- Detach then delete straight away while the old container still runs: refused, naming the service to redeploy. Test in Task 4.
- Two services in different teams: a member of the database's team but not of the service's team cannot attach. Test in Task 4.
- A prod create submitted as a change request: the payload holds no password (the password is generated on execution, not at proposal). Test in Task 4.
- An app user can't read another schema or `platform`: real grant test against MySQL. Test in Task 2.
- A service that already has a database in that environment: attaching a second is refused with a hint naming the current one. Test in Task 4.

## Deviations from the spec

- Connection cap: spec says 10 per user. Plateful runs three services on one database, each with a pool of 10, so the cap is a per-database `max_connections` (default 10, 1–30) set at create (and at adoption in plan 3).
- Backup download is plan 4; this plan's delete has no "download first" step yet.
- The admin account cannot be strictly "not root-like": to grant on arbitrary new schemas it needs `ALL ON *.* WITH GRANT OPTION` plus `CREATE USER`. It is fenced with MySQL 8 partial revokes on `mysql.*` and `platform.*` (Task 8).

---

### Task 1: Storage

**Files:**
- Create: `migrations/000011_databases.up.sql`, `.down.sql`
- Create: `app/database/entities/database.go`, `naming.go`, `naming_test.go`
- Create: `app/database/repositories/mysql/databases.go`, `model.go`, `databases_test.go`
- Modify: `internal/testdb/testdb.go` (`tables` += `databases`, `database_attachments`)

**Interfaces:**
- Produces:
  - `entities.Database{ID, Name, Environment, TeamID, Schema, Username string; PasswordCiphertext, PasswordNonce []byte; MaxConnections int; Adopted bool; CreatedAt, UpdatedAt time.Time}`
  - `entities.Attachment{DatabaseID, ServiceID, Environment string; AttachedAt time.Time; DetachedAt *time.Time}`; `(a Attachment) Active() bool`
  - `entities.ValidateName(name string) error`, `entities.SchemaFor(name, environment string, protected bool) string`, `entities.UsernameFor(schema string) (string, error)`, `entities.ValidateMaxConnections(n int) error`, `entities.Reserved(schema string) bool`
  - `mysql.DatabaseRepository` with `Create(ctx, d) error` (duplicate `(environment,name)` → `apperror.Conflict("a database with this name already exists in this environment")`), `Get(ctx, environment, name) (Database, error)` (not found hint `list databases with GET /api/v1/databases`), `GetByID(ctx, id)`, `List(ctx, environment, teamID string) ([]Database, error)`, `Delete(ctx, id) error` (also deletes its attachments), `Attach(ctx, a Attachment) error`, `Detach(ctx, databaseID, serviceID string, at time.Time) error`, `Attachments(ctx, databaseID string) ([]Attachment, error)` (active and draining), `ActiveFor(ctx, serviceID, environment string) (Database, bool, error)`, `ForgetDrained(ctx, databaseID, serviceID string) error`, `DetachService(ctx, serviceID string, at time.Time) error`.

- [ ] **Step 1: Migration.** `databases(id CHAR(36) PK, name VARCHAR(20), environment VARCHAR(32), team_id CHAR(36), schema_name VARCHAR(64), username VARCHAR(32), password_ciphertext VARBINARY(512), password_nonce VARBINARY(32), max_connections INT NOT NULL DEFAULT 10, adopted BOOLEAN NOT NULL DEFAULT FALSE, created_at/updated_at DATETIME(3), UNIQUE (environment,name), UNIQUE (schema_name), KEY (team_id))`. `database_attachments(database_id CHAR(36), service_id CHAR(36), environment VARCHAR(32), attached_at DATETIME(3), detached_at DATETIME(3) NULL, PRIMARY KEY (database_id, service_id), KEY (service_id, environment))`. Down drops both.
- [ ] **Step 2: Failing tests.** `Test_SchemaFor` (`("my-app","dev",false)`→`my_app_dev`, `("my-app","prod",true)`→`my_app`, `("x","qa",false)`→`x_qa`), `Test_UsernameFor_RefusesOver32`, `Test_ValidateName` (`ab` ok, `a` no, 21 chars no, `Ab` no), `Test_Reserved`. Repo: `Test_Create_DuplicateIsConflict`, `Test_ActiveFor_IgnoresDetached`, `Test_Attachments_KeepsDrainingRows`, `Test_Delete_RemovesAttachments`, `Test_DetachService_StampsEveryActiveRow`.
- [ ] **Step 3: Run** `go test -p 1 ./app/database/...` → FAIL.
- [ ] **Step 4: Implement.**
- [ ] **Step 5: Run** → PASS.
- [ ] **Step 6: Commit** `git commit -m "Store managed databases and their attachments"`

### Task 2: MySQL provisioner

**Files:**
- Create: `app/database/repositories/provisioner/provisioner.go`, `provisioner_test.go`
- Modify: `pkg/config/config.go` (+ `Databases` section), `pkg/config/config_test.go`

**Interfaces:**
- Produces:
  - `config.Databases{AdminDSN string \`mapstructure:"admin_dsn"\`; Host string \`default:"mysql"\`; Port int \`default:"3306"\`}` at `Config.Databases` (env `PLATFORM_DATABASES_ADMIN_DSN`). An empty `AdminDSN` makes every provisioner call return `apperror.Unavailable("managed databases are not configured on this platform")`.
  - `provisioner.New(cfg config.Config) (*Provisioner, error)` (opens lazily; `MaxOpenConns 3`)
  - `(p *Provisioner) SchemaExists(ctx, schema string) (bool, error)`
  - `(p *Provisioner) Create(ctx, schema, user, password string, maxConnections int) error`: `CREATE DATABASE \`schema\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`, `CREATE USER 'user'@'%' IDENTIFIED BY ? WITH MAX_USER_CONNECTIONS n`, `GRANT ALL PRIVILEGES ON \`schema\`.* TO 'user'@'%'`. On a failure after the schema exists, drop what was created and return the first error.
  - `(p *Provisioner) Drop(ctx, schema, user string) error`: `DROP DATABASE IF EXISTS`, `DROP USER IF EXISTS`.
  - Identifiers are interpolated only after matching `^[a-z0-9_]{1,64}$`; anything else is `apperror.Invalid`. The password is a bound parameter.

- [ ] **Step 1: Failing tests** against the test server (`testdb.DSN()` root, schema prefix `kzt_` and cleanup in `t.Cleanup`): `Test_Create_UserSeesOnlyItsSchema` (connect as the new user; `SHOW DATABASES` = `[information_schema, <schema>]`; `CREATE TABLE` inside works; `SELECT` on `platform_test.users` fails with 1142), `Test_Create_CapsConnections` (`SELECT max_user_connections FROM mysql.user` = n), `Test_Create_RollsBackOnUserClash` (pre-create the user → error, schema gone), `Test_Drop_IsIdempotent`, `Test_RefusesUnsafeIdentifiers` (`a;drop`, `` a` ``).
- [ ] **Step 2: Run** `go test -p 1 ./app/database/repositories/provisioner/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `git commit -m "Provision MySQL schemas and scoped users"`

### Task 3: Change requests that name a Resource

**Files:**
- Create: `migrations/000012_request_resources.up.sql`, `.down.sql`
- Modify: `pkg/approval/approval.go`, `app/approvals/entities/request.go`, `app/approvals/repositories/mysql/model.go`, `app/approvals/usecases/submit/submit.go`, `app/approvals/handlers/requests/dto.go`, `app/approvals/entities/event.go`, `app/web/templates/pages/request.html`, `requests.html`
- Test: `app/approvals/usecases/submit/submit_test.go`, `app/approvals/entities/request_test.go`

**Interfaces:**
- Produces: `approval.Change.Resource string` and `entities.Request.Resource string` (e.g. `database plateful`); `(r Request) Target() string` = `ServiceSlug` if set, else `Resource`. Kinds `approval.KindDatabaseCreate = "database.create"`, `KindDatabaseAttach = "database.attach"`, `KindDatabaseDetach = "database.detach"`, `KindDatabaseDelete = "database.delete"`. `RequestDTO.Resource string \`json:"resource,omitempty"\``.

- [ ] **Step 1: Migration.** `ALTER TABLE change_requests ADD COLUMN resource VARCHAR(80) NOT NULL DEFAULT ''`. `service_id`/`service_slug` stay `NOT NULL`; an empty string means "no service".
- [ ] **Step 2: Failing tests.** `Test_Target_FallsBackToResource`; `Test_Submit_SupersedesByResourceSubject` (two pending `database.x` requests with empty service ID, same env → the older one cancelled); `Test_Notice_NamesTheResource`.
- [ ] **Step 3: Run** `go test -p 1 ./app/approvals/...` → FAIL.
- [ ] **Step 4: Implement**; the notice, the audit `TargetName` and both templates use `Target()`.
- [ ] **Step 5: Run** → PASS.
- [ ] **Step 6: Commit** `git commit -m "Let change requests point at a resource"`

### Task 4: Databases usecase

**Files:**
- Create: `app/database/usecases/databases/databases.go` (ports + constructor + reads), `create.go`, `attach.go`, `delete.go`, `approval.go`, `visible.go`, `access.go`, and `_test.go` files with fakes in `fakes_test.go`
- Create: `app/database/repositories/catalog/catalog.go` (services, teams, environments, protection), `app/database/repositories/deploy/deploy.go` (running-deployment check)
- Modify: `app/authz/authz.go`, `app/authz/token.go` (+ `EditDatabase`), `pkg/events/events.go` (+ `TargetDatabase = "database"`)

**Interfaces:**
- Consumes: `entities.*`, `mysql.DatabaseRepository` (Task 1), `*provisioner.Provisioner` (Task 2), `approval.Kind*` / `Change.Resource` (Task 3), `*secretbox.Box` (`Seal`/`Open`, as `credentials.Sealer` does).
- Produces:
  - `databases.Draft{Name, Environment, Team string; MaxConnections int}`
  - `(u DatabasesUsecase) Create(ctx, a, d Draft) (entities.Database, error)`: validate, team via `FindTeam(slug)`, gate `EditDatabase` on `{TeamID, Environment}`; `NeedsApproval` → submit `KindDatabaseCreate` with subject `database.<name>`, `Resource "database <name>"`, payload `Proposal{Kind, Environment, Name, Draft}` (no secrets); else check `Reserved` and `SchemaExists`, generate the password, `Provisioner.Create`, store sealed, publish `database.created`. If the row insert fails after provisioning, `Drop`.
  - `(u) Attach(ctx, a, environment, name, serviceSlug string) (entities.Database, error)`: gate on the database plus `Can(EditSecrets)` on the service's team/env; refuse when `ActiveFor(service, env)` finds another database (`Conflict`, hint `detach <other> first`); a protected env goes through `KindDatabaseAttach`; publish `database.attached` with `{"service": slug, "takes_effect": "next deploy"}`.
  - `(u) Detach(ctx, a, environment, name, serviceSlug string) (entities.Database, error)`: same gate; `KindDatabaseDetach` in a protected env; publish `database.detached`.
  - `(u) Delete(ctx, a, environment, name string) error`: in this order: active attachments → `Conflict("the database still has attached services")` hint `detach these first: <slugs>`; draining attachments whose service is still running → `Conflict("services still run with this database's credentials")` hint `redeploy or remove these first: <slugs>`; drained rows → `ForgetDrained`; then gate (`KindDatabaseDelete` in prod, checked before submitting); then `Provisioner.Drop`, `store.Delete`, publish `database.deleted`.
  - `(u) GetFor`, `ListFor(ctx, a, environment, teamSlug)` (filter `a.Sees`), `AttachmentsFor(ctx, a, environment, name) ([]AttachmentView, error)` with `AttachmentView{Service string; AttachedAt time.Time; DetachedAt *time.Time; Draining bool}`.
  - `(u) Access(ctx, serviceID, environment string) (env, secrets map[string]string, err error)`: empty maps when no active attachment; values per Global Constraints.
  - `(u) DetachService(ctx, a, serviceID string) error` for decommission.
  - `(u) ApplyApproved(ctx, a, o Proposal) (entities.Database, error)`, `(u) Preview(ctx, o Proposal) (approval.Preview, error)`: lines `database`, `schema`, `team`, `service` as relevant.
  - Deploy port: `Running interface { RunsSince(ctx, serviceID, environment string, since time.Time) (bool, error) }`, true when the latest succeeded deployment is not a removal and started at or before `since`. Implemented in `repositories/deploy` with the deploy store's `LatestSucceeded`.

- [ ] **Step 1: Failing tests** (fakes for store, provisioner, catalog, running, authz, requests, publisher):
  - `Test_Create_ProvisionsAndSealsThePassword` (provisioner got schema `shop_dev`, user `kz_shop_dev`, cap 10; stored ciphertext ≠ password; event has no password).
  - `Test_Create_RefusesExistingSchema`, `Test_Create_RefusesReservedSchema` (name `platform` in prod).
  - `Test_Create_ProtectedEnvSubmitsWithoutSecrets` (payload JSON lacks `password`; provisioner not called).
  - `Test_Create_DropsWhenTheRowFails`.
  - `Test_Attach_NeedsRightsOnTheService`, `Test_Attach_OneDatabasePerServiceAndEnv`.
  - `Test_Delete_RefusesWhileAttached`, `Test_Delete_RefusesWhileDetachedServiceStillRuns`, `Test_Delete_ForgetsDrainedAttachments`, `Test_Delete_AttachedCheckedBeforeApproval`.
  - `Test_Access_InjectsTheSixValues` (exact map), `Test_Access_EmptyWithoutAttachment`, `Test_Access_IgnoresDetached`.
  - `Test_RunsSince` in the deploy adapter: removal → false; started after `since` → false; started before → true; none → false.
- [ ] **Step 2: Run** `go test -p 1 ./app/database/... ./app/authz/...` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `git commit -m "Create, attach, detach and delete managed databases"`

### Task 5: Credentials at deploy, reserved names, service deletion

**Files:**
- Create: `app/deploy/repositories/database/database.go`
- Modify: `app/deploy/usecases/agents/agents.go`, `claim.go` (second port `DatabaseAccess`, merged after queue access), `app/envconfig/entities/validate.go`, `app/catalog/usecases/decommission/decommission.go` (+ `Databases` port, called after `queues.Detach`), `cmd/controlplane/modules/deploy.go`, `catalog.go`
- Test: `app/deploy/usecases/agents/agents_test.go`, `app/envconfig/entities/entities_test.go`, `app/catalog/usecases/decommission/decommission_test.go`

**Interfaces:**
- Consumes: `DatabasesUsecase.Access`, `DatabasesUsecase.DetachService` (Task 4).
- Produces: `agents.DatabaseAccess interface { DatabaseAccess(ctx, serviceID, environment string) (env, secrets map[string]string, err error) }`; `NewAgentsUsecase` gains a `databases DatabaseAccess` parameter after `queues`.

- [ ] **Step 1: Failing tests.** `Test_Claim_MergesDatabaseAccess` (secrets contain `PLATFORM_DB_PASSWORD`, platform value wins over a hand-set one), `Test_ValidateName_ReservesDatabaseNames` (all six), `Test_Finish_DetachesDatabases`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `go test -p 1 ./app/deploy/... ./app/envconfig/... ./app/catalog/...` → PASS.
- [ ] **Step 5: Commit** `git commit -m "Inject database credentials at deploy"`

### Task 6: API, MCP, resources source, approvals executor

**Files:**
- Create: `app/database/handlers/databases/databases.go`, `dto.go`, `operations.go`
- Create: `app/resources/repositories/database/database.go`
- Create: `app/approvals/repositories/database/database.go`
- Create: `cmd/controlplane/modules/database.go`; modify `approvals.go` (executors + `submitInjections`), `resources.go` (source), the fx app
- Test: `internal/integration/tests/databases_test.go`, `mcp_test.go`

**Interfaces:**
- Operations (all `Gated` except reads; reads scope `read`, writes scope `services`):
  `listDatabases GET /databases` (`environment`, `team`), `createDatabase POST /databases` (`{name, environment, team, max_connections?}`, 201), `getDatabase GET /databases/:env/:name`, `attachDatabase POST /databases/:env/:name/attachments` (`{service}`), `detachDatabase DELETE /databases/:env/:name/attachments/:service`, `deleteDatabase DELETE /databases/:env/:name` (204). `deleteDatabase` is **not** added to the MCP `excluded` list (spec §9).
  `DatabaseDTO{ID, Name, Environment, TeamID, Host, Port, Schema, Username, MaxConnections, Adopted, Attachments []AttachmentDTO{Service, AttachedAt, DetachedAt, Draining}, Note string, CreatedAt, UpdatedAt}`; `Note` = `"takes effect on each service's next deploy"` on attach/detach responses. No password field exists.
- Resources source: `Attached` = slugs of active attachments.

- [ ] **Step 1: Failing integration tests** (provisioner configured with the test root DSN in the suite; extend `integration.NewSuite` config): `Test_DatabaseLifecycleInDev` (create → attach `places-api` → deploy claim carries `PLATFORM_DB_DSN` that opens a working connection → delete refused → detach → delete refused while running → redeploy succeeds → delete 204 → schema gone), `Test_DatabaseInProdNeedsApproval` (maintainer create → 202 pending; approve → executed; schema exists), `Test_DatabaseNeverReturnsThePassword` (no response body contains the stored password), `Test_ResourcesListDatabases`, `Test_McpListsDatabaseTools`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `go test -p 1 ./...` → PASS, `make lint` clean.
- [ ] **Step 5: Commit** `git commit -m "Expose managed databases in the API and MCP"`

### Task 7: Database pages (mockups first)

**Files:**
- Create: `app/web/handlers/databases/databases.go`, `actions.go`; `app/web/templates/pages/database.html`, `database_new.html`
- Modify: `resources.html` / `resource_new.html` (database kind enabled), `service.html` + `app/web/handlers/service/service.go` (attached resources card), `cmd/controlplane/modules/web.go`
- Test: `internal/integration/tests/database_pages_test.go`

**Interfaces:**
- Routes: `GET /resources/new/database`, `POST /resources/databases`, `GET /resources/databases/:env/:name`, `POST .../attach`, `POST .../detach/:service`, `POST .../delete`.

- [ ] **Step 1: Mockups.** An artifact with options for: the database page (connection facts without the password, the **Attached services** table with per-row detach and a "draining" state, attach control, blocked delete with its reason), the create form, and the service page's attached-resources card. Wait for the user's pick.
- [ ] **Step 2: Failing tests.** `Test_DatabasePageShowsFactsNotPassword`, `Test_DatabaseDeleteDisabledWhileAttached` (button disabled, text names the service), `Test_ServicePageListsAttachedResources`.
- [ ] **Step 3: Run** → FAIL. **Step 4: Implement** per the chosen mockup. **Step 5: Run** `go test -p 1 ./...` → PASS; check in a browser in light and dark.
- [ ] **Step 6: Commit** `git commit -m "Show and manage databases in the web UI"`

### Task 8: Infra (kalzone-infra)

**Files:**
- Modify: `stack/compose.data.yml` (mysql `--partial_revokes=ON`), `stack/scripts/up.sh` (+ `ensure_provisioner_user`), `terraform/variables.tf` (+ `databases_admin_password`, same validation as `platform_db_password`), `terraform/locals.tf`, `terraform/templates/stack.env.tftpl`, `terraform/templates/controlplane.secrets.env.tftpl` (+ `PLATFORM_DATABASES_ADMIN_DSN='kalzone_provisioner:${databases_admin_password}@tcp(mysql:3306)/'`)

- [ ] **Step 1:** `ensure_provisioner_user` runs: `CREATE USER IF NOT EXISTS 'kalzone_provisioner'@'%' IDENTIFIED BY '…' WITH MAX_USER_CONNECTIONS 3; ALTER USER …; GRANT ALL PRIVILEGES ON *.* TO 'kalzone_provisioner'@'%' WITH GRANT OPTION; REVOKE ALL PRIVILEGES ON mysql.* FROM 'kalzone_provisioner'@'%'; REVOKE ALL PRIVILEGES ON platform.* FROM 'kalzone_provisioner'@'%';`
- [ ] **Step 2:** `make test` in kalzone-infra (the up.sh shell tests) → PASS; extend them to assert the new function runs with `WITH_DATA=1`.
- [ ] **Step 3:** Back up tfvars to `~/workspace/kalzone/backups/terraform.tfvars.pre-databases-2026-10-19`, add the generated password, `terraform apply` (whole repo mounted, per the VPS memory), check on the VPS that `SHOW GRANTS FOR kalzone_provisioner` lists the partial revokes and that it cannot `SELECT` from `platform.users`.
- [ ] **Step 4: Commit** in kalzone-infra `git commit -m "Give the control plane a MySQL provisioner account"`

### Task 9: Ship

- [ ] **Step 1:** Rewrite the kalzone branch with layered-commits (storage → provisioner → rules → handlers+wiring+integration).
- [ ] **Step 2:** Push both repos, open PRs (infra first), merge with rebase, apply infra, release and deploy the control plane.
- [ ] **Step 3:** Smoke test on the live platform with the `hello` app in dev: create `hello` database, attach, deploy, check the app's health with the DB check, detach, redeploy, delete.

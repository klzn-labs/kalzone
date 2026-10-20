# Resources 3: Adoption and Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the hand-made Plateful and LegendsGrid schemas under Resources in place, switch the apps to `PLATFORM_DB_*`, and let deleting an adopted database release it instead of dropping it.

**Architecture:** The provisioner is fenced off every existing schema and cannot lift its own fence, so adoption has a root step: an infra script, run as root on the VPS, creates a locked `kz_<schema>` user with `ALL ON <schema>.*`. That grant row is also the "managed" marker, so later up.sh runs stop fencing the schema. The platform's admin-only adopt call then sets that user's password and connection limit with `ALTER USER`, proves the user can see the schema, records the database with `adopted = true` and attaches the services. Release drops only the `kz_` user and the record.

**Tech Stack:** Go (gaardrail layout, Uber fx, gorm, go-sql-driver/mysql), MySQL 8.0 with partial_revokes, bash (kalzone-infra), viper (go-plateful, legendsgrid).

**Spec:** docs/superpowers/specs/2026-10-19-resources-design.md (section 7 Adoption; anti-goals in section 3)

## Global Constraints

- No comments in any file. Function bodies ≤ 20 lines, ≤ 2 levels of nesting. Commits authored as the user, no attribution trailers.
- Adoption is platform-admin only. New action `AdoptDatabase`: token scope `services:admin`, not protectable (no change request), no team role grants it.
- Adoption never alters, renames, locks or drops the existing schema and never touches the old user (`plateful`, `lolgrid`, `lolgrid_dev`). Running containers keep their old credentials until redeployed.
- Adopted user: `kz_<schema>` (≤ 32 characters, as `entities.UsernameFor`), grants `ALL PRIVILEGES ON \`<schema>\`.*` only, `MAX_USER_CONNECTIONS` = the database's `max_connections` (1..30).
- Deleting an adopted database releases it: drop the `kz_` user and the record, leave schema and data. The UI labels the action "Release" and says the data stays; the API and MCP response say the same. The attached / still-running blocks apply as for any delete.
- A password never appears in an API response, page, log line, audit detail or change request payload.
- Apps read `PLATFORM_DB_HOST`, `PLATFORM_DB_PORT`, `PLATFORM_DB_NAME`, `PLATFORM_DB_USER`, `PLATFORM_DB_PASSWORD`. No name mapping in the platform. Hand-set `APP_MYSQL_*` values are removed only after the switched version runs in that environment; `APP_MYSQL_MAXOPENCONNS` stays (it is a pool size, not a credential).
- Dropping the old shared users is a separate manual admin step — not in this plan.
- Every adopt and release writes an audit event without secret values.

## Review Focus

- Root step not run yet (no `kz_<schema>` user): adopt must fail with a Conflict whose hint names the exact infra command, and leave no record. → Task 2, Task 3 tests.
- A `kz_<schema>` user that exists but has no grant on the schema (e.g. made by the provisioner itself): adopt must refuse, and leave the user locked. → Task 2 test.
- Adopting a schema that a managed record already uses, a reserved schema, or one that does not exist: refused before any MySQL change. → Task 3 tests.
- Release while a service is attached or still running with the credentials: refused exactly like delete, schema untouched in every path. → Task 3 tests.
- An app deployed with the switched config in an environment where no database is attached gets no `PLATFORM_DB_*` and must fail fast at startup (validation), not run against defaults. → Tasks 6–7 tests.

---

### Task 1: Root adoption step (kalzone-infra)

**Files:**
- Create: `stack/scripts/adopt-schema.sh`
- Modify: `README.md` (provisioner section: how to adopt and release)
- Test: `test/provisioner.sh`

**Interfaces:**
- Produces: `stack/scripts/adopt-schema.sh <schema>`, run as root on the VPS from `/opt/plateful/stack` (`ssh root@<vps> /opt/plateful/stack/scripts/adopt-schema.sh plateful`). It execs into the mysql container like up.sh does. Exit 0 also when it has already been done (idempotent).

- [ ] **Step 1: Write the failing tests** in `test/provisioner.sh`:
  - `check_adopt_schema`: seed schema `appx` with a table and the old account `appx_old`. Run `adopt-schema.sh appx`, then the ensure run. Assert:
    - `kz_appx`@'%' exists, is locked, and holds `ALL PRIVILEGES ON appx.*` and nothing else;
    - the provisioner can now `ALTER USER 'kz_appx'@'%' IDENTIFIED BY 'p' ACCOUNT UNLOCK`, and `kz_appx` can SELECT the table;
    - `appx` is no longer in the provisioner's partial revokes (managed);
    - `appx_old`'s grants are unchanged and the table's rows are unchanged.
  - `check_adopt_refusals`: `adopt-schema.sh` exits non-zero and changes nothing for: `mysql`, `sys`, `platform`, `information_schema`, `performance_schema`, a schema that does not exist, a name not matching `^[a-z0-9_]{1,29}$`, and no argument.
  - `check_release_refences`: after the provisioner runs `DROP USER 'kz_appx'@'%'`, the next ensure run fences `appx` again, and `appx` and its rows still exist.
  - A second `adopt-schema.sh appx` run exits 0 and changes nothing.
- [ ] **Step 2: Run** `bash test/provisioner.sh`. Expected: FAIL (script missing).
- [ ] **Step 3: Implement `adopt-schema.sh`.** Validate the argument against `^[a-z0-9_]{1,29}$` and the reserved list. As root in the container, check the schema exists in `information_schema.SCHEMATA` (exact, binary compare). Then: `CREATE USER IF NOT EXISTS 'kz_<schema>'@'%' IDENTIFIED BY '<64 random hex>' ACCOUNT LOCK; GRANT ALL PRIVILEGES ON \`<schema>\`.* TO 'kz_<schema>'@'%';`. The random password is never printed or stored; the platform replaces it.
- [ ] **Step 4: Run** `bash test/provisioner.sh` twice and `make test`. Expected: `provisioner ok` both times.
- [ ] **Step 5: Commit** `Add a root step that prepares a schema for adoption`.

### Task 2: Provisioner adopt and release (kalzone)

**Files:**
- Modify: `app/database/repositories/provisioner/provisioner.go`
- Test: `app/database/repositories/provisioner/provisioner_test.go`

**Interfaces:**
- Produces:
  - `func (p *Provisioner) Adopt(ctx context.Context, schema, user, password string, maxConnections int) error`
  - `func (p *Provisioner) Release(ctx context.Context, user string) error`
  - `var errNotPrepared = apperror.Conflict("this schema has not been prepared for adoption").WithHint("as root on the VPS run /opt/plateful/stack/scripts/adopt-schema.sh <schema>, then retry")`, with the real schema name substituted for `<schema>`.

- [ ] **Step 1: Write the failing tests** (local test MySQL; the test plays the root step with the root connection; schemas prefixed `kzt_`, dropped in `t.Cleanup`):
  - `Test_Adopt_SetsThePasswordAndUnlocks`: after the root step on `kzt_ad` (which holds table `t` with one row), `Adopt(ctx, "kzt_ad", "kz_kzt_ad", "pw", 7)` returns nil. A connection as `kz_kzt_ad`/`pw` reads the row, `MAX_USER_CONNECTIONS` is 7, and the account is unlocked.
  - `Test_Adopt_RefusesWithoutTheRootStep`: no `kz_` user. The returned error has `apperror.CodeConflict` and a hint containing `adopt-schema.sh kzt_ad`.
  - `Test_Adopt_RefusesAUserWithoutTheGrant`: `kz_kzt_ad` exists without a grant. Expect a Conflict, and the account is locked afterwards.
  - `Test_Adopt_LeavesTheSchemaAlone`: after Adopt, the table definitions and rows of `kzt_ad` are unchanged.
  - `Test_Release_DropsOnlyTheUser`: after Adopt then `Release(ctx, "kz_kzt_ad")`, the user is gone and the schema and rows remain. Release of a missing user returns nil.
  - `Test_Release_RefusesANonPlatformUser`: `Release(ctx, "plateful")` returns `CodeInvalid` (the user must start with `kz_` and match the identifier rule).
- [ ] **Step 2: Run** `go test -p 1 -count=1 ./app/database/repositories/provisioner/`. Expected: FAIL (undefined methods).
- [ ] **Step 3: Implement.**
  - **Adopt:** run `check(schema, user)` and the password/maxConnections checks used by Create. Then `ALTER USER '<user>'@'%' IDENTIFIED BY ? WITH MAX_USER_CONNECTIONS n ACCOUNT UNLOCK`. MySQL 1396 (no such user) maps to `errNotPrepared`.
  - **Proof:** open a short-lived connection with the admin DSN's address, user/password replaced and DBName = schema. Require that `SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?` is 1.
  - **Failing proof:** if the connection or the count fails, run `ALTER USER ... ACCOUNT LOCK` (bounded by `rollbackTimeout`) and return `errNotPrepared`.
  - **Release:** check that user matches `identifier`, starts with `entities.UsernamePrefix` and is not reserved, then run `DROP USER IF EXISTS '<user>'@'%'`. Never touches a schema.
- [ ] **Step 4: Run** the package tests. Expected: PASS.
- [ ] **Step 5: Commit** `Adopt an existing schema's prepared user and release it`.

### Task 3: Adopt and release rules (kalzone)

**Files:**
- Modify: `app/authz/authz.go`, `app/authz/token.go`
- Create: `app/database/usecases/databases/adopt.go`
- Modify: `app/database/usecases/databases/databases.go` (Provisioner port), `delete.go`, `fakes_test.go`
- Test: `app/authz/authz_test.go`, `app/authz/token_test.go`, `app/database/usecases/databases/adopt_test.go`, `delete_test.go`

**Interfaces:**
- Consumes: Task 2's `Adopt` and `Release` (added to the `Provisioner` port).
- Produces:
  - `authz.AdoptDatabase Action = "database.adopt"`: scope `scope.ServicesAdmin`, not in `protectable`. `requiredRole` returns `RoleNone` for it, so only admins pass.
  - `type AdoptDraft struct { Schema, Name, Environment, Team string; MaxConnections int; Services []string }`, with JSON tags `schema,name,environment,team,max_connections,services`.
  - `func (u DatabasesUsecase) Adopt(ctx context.Context, a actor.Actor, d AdoptDraft) (entities.Database, error)`.
  - `Delete` keeps its signature. For an adopted row it releases instead of dropping and publishes `database.released`; it never calls `Provisioner.Drop` for an adopted row. `errAdopted` is removed.

- [ ] **Step 1: Write the failing tests** (fakes as in the existing usecase tests):
  - `Test_Adopt_NeedsAnAdmin`: a maintainer gets `CodeForbidden`, and the fake provisioner records no call.
  - `Test_Adopt_RecordsAndAttaches`: admin, `{Schema:"plateful", Name:"plateful", Environment:"prod", Team:"plateful", MaxConnections:30, Services:["api","worker","importer"]}`. The result has `Adopted:true`, `Schema:"plateful"`, `Username:"kz_plateful"`, `MaxConnections:30`, and three active attachments. The fake got `Adopt("plateful","kz_plateful",<non-empty>,30)`. One `database.adopted` event is published, and its detail holds schema and services but no password.
  - `Test_Adopt_Refuses` (table test). Each case is refused with no provisioner call and no record:
    - a reserved schema (`platform`) → `CodeInvalid`;
    - a schema another record uses → `CodeConflict`;
    - `SchemaExists` false → `CodeNotFound`, hint "check the schema name in MySQL";
    - an invalid name → `CodeInvalid`;
    - a name taken in the environment → `CodeConflict`;
    - an unknown service → `CodeNotFound`;
    - `MaxConnections` 31 → `CodeInvalid`.
  - `Test_Adopt_KeepsNoRecordWhenTheProvisionerRefuses`: the fake `Adopt` returns a Conflict, so no record is stored and the error is passed through.
  - `Test_Adopt_DefaultsMaxConnections`: 0 becomes 10.
  - `Test_Delete_ReleasesAnAdoptedDatabase`: the fake records `Release("kz_plateful")` and no `Drop`. The row is gone and one `database.released` event is published.
  - `Test_Delete_RefusesReleaseWhileAttachedOrRunning`: the same conflicts as delete, with no provisioner call.
  - `Test_ApplyApproved_ReleasesAnAdoptedDatabase`: an approved `KindDatabaseDelete` proposal for an adopted row releases it.
  - authz: `AdoptDatabase` is allowed for an admin session and for an admin's token with `services:admin`. It is refused for a token with only `services` (`CodeMissingScope`) and for a maintainer.
- [ ] **Step 2: Run** `go test -p 1 -count=1 ./app/authz/ ./app/database/usecases/databases/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - **Adopt order:** validate the draft, then `u.authz.Gate(ctx, a, authz.AdoptDatabase, resource)` through `authz.Direct`. Next run the reserved check, the schema-unused-by-any-record check, `SchemaExists`, the name-free check, and resolve the team and every service. Then seal the password and call `provisioner.Adopt`.
  - **After provisioning:** `store.Create` with `Adopted: true` (on store failure, `provisioner.Release` bounded by `undoTimeout`, then return the error). Store one attachment per service using the existing attach storage path, then publish `database.adopted`.
  - **Release** is a `release` method chosen in `drop` when `db.Adopted`.
- [ ] **Step 4: Run** both packages. Expected: PASS.
- [ ] **Step 5: Commit** `Adopt existing schemas and release adopted databases`.

### Task 4: Adopt in the API and MCP (kalzone)

**Files:**
- Modify: `app/database/handlers/databases/databases.go`, `dto.go`, `operations.go`, `cmd/controlplane/modules/database.go` (if the handler's port grows)
- Test: `internal/integration/tests/databases_test.go`, `internal/integration/tests/mcp_test.go`

**Interfaces:**
- Consumes: Task 3's `Adopt` and `Delete`.
- Produces:
  - `POST /api/v1/databases/adopt`, operation `adoptDatabase`, body `adoptDTO` (`schema`, `name`, `environment`, `team`, `max_connections`, `services`). It answers 201 with `DatabaseDTO` and `note: "services get the credentials on their next deploy; the schema was not changed"`.
  - `DELETE /api/v1/databases/:env/:name`: for an adopted row it answers 200 `{"released": true, "message": "the platform's user and record are removed; the schema and its data stay in MySQL"}`. For a managed row it stays 204.

- [ ] **Step 1: Write the failing integration tests** (real provisioner on the test MySQL; the test plays the root step as root):
  - `Test_AdoptDatabase_EndToEnd`: root step on `kzt_old` (table with a row). An admin token posts adopt with services `[<test service>]` and gets 201 with `adopted:true`, `username:"kz_kzt_old"` and no `password` key. A deployment claim for that service and environment receives a `PLATFORM_DB_DSN` that opens and reads the row.
  - `Test_AdoptDatabase_WithoutRootStep`: 409 with `hint` containing `adopt-schema.sh kzt_old`, and `GET /databases` stays empty.
  - `Test_AdoptDatabase_NeedsServicesAdmin`: a token with `services` but not `services:admin` gets 403 `err_missing_scope`.
  - `Test_ReleaseDatabase_KeepsTheData`: after detach and drain, DELETE answers 200 `released:true`. The schema and row remain and the `kz_` user is gone.
  - `Test_McpListsDatabaseTools` asserts `adoptDatabase` is present, and its summary says platform admins only and that the root step comes first.
- [ ] **Step 2: Run** `go test -p 1 -count=1 ./internal/integration/...`. Expected: FAIL.
- [ ] **Step 3: Implement** the handler, DTO and operation, following the `createDatabase` pattern. The operation summary reads: "Platform admins only. Bring an existing MySQL schema under the platform in place after the root step (adopt-schema.sh) has prepared it; attaches the services, changes no data".
- [ ] **Step 4: Run** the full suite `go test -p 1 -count=1 ./...` and `make lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Adopt databases through the API and MCP`.

### Task 5: Show adopted databases and release them in the web UI (kalzone)

**Files:**
- Modify: `app/web/templates/pages/database.html`, `app/web/templates/pages/resources.html`, the database page view model in `app/web/handlers/...` (where `CanEdit` was added), `app/web/handlers/.../actions.go` (the delete action's redirect/flash)
- Test: the web handler tests next to `Test_DatabaseDeleteEnabledWhenNothingIsAttached`

**Interfaces:**
- Consumes: Task 3's `Delete` (releases adopted rows).

- [ ] **Step 1: Write the failing tests:**
  - `Test_DatabasePageLabelsAnAdoptedDatabase`: the page shows an `Adopted` badge (reuse the existing badge component and classes). The danger zone button reads `Release`, and the copy reads "Removes the platform's user and record. The schema `<schema>` and its data stay in MySQL." The typed-name confirm is still required.
  - `Test_DatabaseReleaseKeepsTheSchema`: posting the release with the typed name answers like delete. The fake provisioner records `Release`, not `Drop`, and the flash reads "Released <name>; its data stays in MySQL."
  - `Test_ResourcesListMarksAdoptedRows`: the `/resources` database row shows the `Adopted` badge.
- [ ] **Step 2: Run** the web handler package tests. Expected: FAIL.
- [ ] **Step 3: Implement** with the existing components only. No new layout, no adopt form; adoption is API/MCP for admins.
- [ ] **Step 4: Run** the full suite and `make lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Label adopted databases and release them from the web UI`.

### Task 6: go-plateful reads PLATFORM_DB_* (adrianozp/go-plateful)

**Files:**
- Modify: `pkg/config/config.go` (`bindEnvVars`), `docker-compose.yml`, `docker-compose.vps.yml`, `.github/workflows/go.yml`, any README/env example that names `APP_MYSQL_*`
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `mysql.host`←`PLATFORM_DB_HOST`, `mysql.port`←`PLATFORM_DB_PORT`, `mysql.database`←`PLATFORM_DB_NAME`, `mysql.username`←`PLATFORM_DB_USER`, `mysql.password`←`PLATFORM_DB_PASSWORD` (via `viper.BindEnv(key, envName)`). `mysql.maxopenconns` stays `APP_MYSQL_MAXOPENCONNS`. The five credential keys are no longer read from `APP_MYSQL_*`.

- [ ] **Step 1: Write the failing tests:**
  - `TestConfig_ReadsPlatformDatabaseValues`: setting the five `PLATFORM_DB_*` values (port `7171`) yields those values in `cfg.MySQL`.
  - `TestConfig_IgnoresOldMySQLNames`: setting only `APP_MYSQL_USERNAME=old` leaves `cfg.MySQL.Username` at its config-file/default value, not `old`.
  - `TestConfig_KeepsMaxOpenConns`: `APP_MYSQL_MAXOPENCONNS=4` still gives 4.
  - Replace the old `APP_MYSQL_*` assertions in `config_test.go`.
- [ ] **Step 2: Run** `go test ./pkg/config/`. Expected: FAIL.
- [ ] **Step 3: Implement** in `bindEnvVars`, and rename the variables in the compose files and CI workflow.
- [ ] **Step 4: Run** the repo's full test command (as in `.github/workflows/go.yml`) and its linter. Expected: PASS.
- [ ] **Step 5: Commit** `Read database credentials from PLATFORM_DB_*`.

### Task 7: legendsgrid reads PLATFORM_DB_* (adrianozp/legendsgrid)

**Files:**
- Modify: `pkg/config/mysql.go` (bindings), `pkg/config/config.go` if `bindEnv` needs per-key env names, `config.yaml`, `docker-compose.yml`, CI workflow and README mentions
- Test: `pkg/config/config_test.go` (create if missing)

**Interfaces:**
- Produces: the same five bindings as Task 6. `mysql.maxopenconns` stays `APP_MYSQL_MAXOPENCONNS`.

- [ ] **Step 1: Write the failing tests:** the same three as Task 6 (`Test_Config_ReadsPlatformDatabaseValues`, `Test_Config_IgnoresOldMySQLNames`, `Test_Config_KeepsMaxOpenConns`), plus `Test_Config_FailsWithoutDatabaseUser`: with no user configured anywhere, `Load` returns a validation error.
- [ ] **Step 2: Run** `go test ./pkg/config/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Use a per-key env map in `mysql.go` (`map[string]string{"mysql.host": "PLATFORM_DB_HOST", ...}`) bound in `bindEnv` with `viper.BindEnv(key, env)`. The other packages keep `envKeys`.
- [ ] **Step 4: Run** the repo's full tests and linter. Expected: PASS.
- [ ] **Step 5: Commit** `Read database credentials from PLATFORM_DB_*`.

### Task 8: Roll out on the live platform (controller, not dispatched)

Each step's result is checked before the next. Prod steps follow the standing ship rules; the token bypasses change requests.

- [ ] **Step 1: Ship code.** The infra PR, then the kalzone PR (layered commits, CI green, rebase-merge). Run the infra apply (stack reinstall, no mysql restart) and the control plane release.
- [ ] **Step 2: Run the root step for every schema:** `ssh root@89.117.146.29 /opt/plateful/stack/scripts/adopt-schema.sh <schema>` for `plateful_dev`, `lolgrid_dev`, `plateful`, `lolgrid`.
- [ ] **Step 3: Adopt, dev first.** Use `max_connections: 30` for each:
  - `{schema:"plateful_dev", name:"plateful", environment:"dev", team:"plateful", services:["api","worker","importer"]}`
  - `{schema:"lolgrid_dev", name:"lolgrid", environment:"dev", team:"league-grid", services:["legendsgrid"]}`
- [ ] **Step 4: Ship the app PRs (Tasks 6–7) and deploy them to dev.** Check that each container's env has `PLATFORM_DB_*`, that the app is healthy, and that its logs show DB access. Then remove the hand-set `APP_MYSQL_HOST/PORT/DATABASE/USERNAME` env values and the `APP_MYSQL_PASSWORD` secret in dev (for legendsgrid also the `APP_MYSQL_DATABASE`/`APP_MYSQL_USERNAME` secrets), and redeploy once more.
- [ ] **Step 5: Adopt in prod:**
  - `plateful` → `{name:"plateful", environment:"prod", team:"plateful", services:["api","worker","importer"]}`
  - `lolgrid` → `{name:"lolgrid", environment:"prod", team:"league-grid", services:["legendsgrid"]}`
  Deploy, verify and clean up as in Step 4. Back up each prod schema first (`mysqldump --single-transaction` as root into deploy@ `~/db-backups/`).
- [ ] **Step 6: Re-run up.sh (infra apply)** and confirm that `plateful`, `plateful_dev`, `lolgrid` and `lolgrid_dev` are no longer in the provisioner's partial revokes, that the old users still exist, and that the apps still serve.
- [ ] **Step 7: Leave** the old users and their `protected-accounts` lines in place. Tell the user that dropping `plateful`, `lolgrid` and `lolgrid_dev` is their manual step.

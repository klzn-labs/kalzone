# Resources — design

Date: 2026-10-19
Status: agreed with the user in conversation on 2026-10-19, waiting for review of this document

## 1. Why

Kalzone manages queues end to end: a team creates one, attaches services to it, and the
platform injects `PLATFORM_QUEUE_*` credentials at deploy. Databases get none of that.
There is one shared MySQL 8 (`mysql:3306`), and the skill docs say "there is no
self-service database, ask the admin". Every database so far was made by an agent by
hand: `CREATE DATABASE`, a user, grants, then the password pasted into each service with
`setSecret`. The platform does not know a database exists, who owns it, or who uses it.
Plateful's `api`, `worker` and `importer` all connect with the compose-level `plateful`
user, which has more rights than they need.

The goal is that anyone onboarding an app (the user in the UI, an agent through the
`kalzone` MCP tools, or a friend onboarding their own app) can get a database and wire it to
a service without the platform admin, and never handle a password.

## 2. Decisions the user made

- A new top-level section named **Resources** groups every managed backing resource.
  Queues move into it, databases are the second kind, and more kinds can follow.
  ("Services" already names the apps and stays as it is.)
- All three paths are first-class: UI, HTTP API, MCP tools.
- A **database** is, for now, a schema on the existing shared MySQL with its **own user
  granted rights only on that schema**.
- A database is **selected for a service** and its credentials reach the service
  **automatically** at deploy.
- **Backwards compatible**: existing hand-made databases (Plateful, LegendsGrid) can be
  brought under Resources without losing data.
- **Download backup**: a `mysqldump`-style export of one database, downloadable.
- **Delete drops the database.**
- **A Resource with attached services cannot be deleted.** Its page lists the attached
  services, and delete stays blocked until that list is empty.
- Who may create: not a priority. Follow the existing queue rules (below).
- **Adopted databases are released, not dropped.** Delete drops schemas the platform
  created; for an adopted one it removes the platform's user and record and leaves the
  schema untouched.
- **Apps switch to `PLATFORM_DB_*`.** No name mapping in the platform. Adoption includes
  the app changes in go-plateful and legendsgrid, and the Go service template and skills
  move from `APP_MYSQL_*` to `PLATFORM_DB_*`.

## 3. Goals, non-goals, anti-goals

Goals:

- Create, list, view, attach, detach, back up and delete a database from the UI, the API
  and MCP, with the same rules everywhere.
- A service with a database attached gets working credentials on its next deploy, with
  no manual secret.
- Adopt existing databases in place.
- One Resources section where queues and databases look and behave alike.

Non-goals (this round):

- Other engines (Postgres, Redis) or a server per database. The model has a `kind` so
  they can come later; nothing is built for them now.
- Read-only attachments, per-service users on a shared database, credential rotation,
  scheduled or stored backups, restore from a dump, quotas on disk size.
- Changing how the whole-server backups in `kalzone-infra` work.

Anti-goals (a failure even if it works):

- A password shown in chat, in the UI, in logs, in audit payloads or in an API response.
- An app user that can see or touch another schema, the `platform` schema or `mysql.*`.
- Adoption that drops, renames or locks the existing schema, or breaks a running service
  before it is redeployed.
- Deleting a database that a service still uses.
- Queues behaving differently from before for apps (gateway, SDK, env names unchanged).

## 4. The model

A **Resource** has: `kind` (`queue` or `database`), `name`, `environment`, owning
`team`, and **attachments** to services in the same environment. Queues keep their
existing tables and code. "Resources" is a grouping in the UI, the API and MCP plus the
shared rules below, not a rewrite of the queue domain into a generic table. *(My call:
the builder may introduce a thin shared layer for listing and the delete rule, but must
not migrate queue storage.)*

### Database

- Fields: id, name (same slug rules as queues), environment, team, MySQL schema name,
  MySQL username, encrypted password (`internal/secretbox`, like queue credentials),
  `adopted` flag, created/updated times.
- Unique per `(environment, name)`.
- Schema name for a new database: `<name>_dev` in dev, `<name>` in prod, the convention
  the skill docs already use. *(My call.)* Creation is refused if that schema already
  exists in MySQL, or is one of `mysql`, `sys`, `information_schema`,
  `performance_schema`, `platform`. An existing schema can only enter through adoption.
- MySQL user: `kz_<name>_<env>` *(my call)*, `GRANT ALL ON <schema>.*` and nothing else,
  `MAX_USER_CONNECTIONS 10` *(my call; the server allows 50)*, password random 32 bytes
  hex.
- Attachments: many services per database (Plateful's three share one). Every attached
  service gets the same user and full read-write access.
- **At most one database attached per service per environment.** *(My call, to keep
  the injected names fixed; see the open question in section 10.)*

### Queue

Unchanged in storage and behaviour, except for the delete rule in section 6 and its
place in the UI.

## 5. Credentials at deploy

`AgentsUsecase.withQueueAccess` (`app/deploy/usecases/agents/claim.go`) already merges
platform values into the resolved config at claim time. Databases add a second source
the same way. For a service with a database attached in that environment:

| Variable | Kind | Value |
|---|---|---|
| `PLATFORM_DB_HOST` | env | `mysql` |
| `PLATFORM_DB_PORT` | env | `3306` |
| `PLATFORM_DB_NAME` | env | schema name |
| `PLATFORM_DB_USER` | secret | MySQL username |
| `PLATFORM_DB_PASSWORD` | secret | password |
| `PLATFORM_DB_DSN` | secret | `user:pass@tcp(mysql:3306)/schema?parseTime=true` (Go driver form) |

*(My call on the names.)* They join `reserved` in `app/envconfig/entities/validate.go`,
so nobody can set them by hand. Platform values win on merge, as with queues.

Attach and detach change nothing in a running container; they take effect on the next
deploy. The UI and the API response say so. *(My call: no automatic redeploy.)*

Existing apps (Plateful `api`/`worker`/`importer`, LegendsGrid) read
`APP_MYSQL_HOST/PORT/DATABASE/USERNAME/PASSWORD`. They keep working on their hand-set
values until their code reads `PLATFORM_DB_*` instead; that code change is part of
adoption (section 7). The platform does not map its values onto other names.

## 6. Rules shared by every Resource

- **Authorization**: the queue rules. A team member may create, attach, detach and delete
  in `dev`. In a protected environment (`prod`) the action needs a maintainer and goes
  through a change request (`pkg/approval`), with a preview, exactly as queue create and
  delete do. Platform admins can do everything. Attaching also requires the actor to be
  allowed to change the target service. *(My call.)*
- **Delete is refused while any service is attached.** The API returns a conflict whose
  hint names the attached services and says to detach them first; the UI disables the
  button with the same text. For a database: drop the schema (only if the platform
  created it, see section 7), drop the user, delete the row, write an audit event.
- **Detached but still running counts as attached.** Detach takes effect on the next
  deploy, so delete also stays blocked while any service's currently running deployment
  in that environment was resolved with this Resource's credentials. The hint names
  those services and says to redeploy or remove them. *(My call, from the review.)* The
  same applies to queue publishers.
- **Change requests without a service.** `change_requests.service_id` becomes nullable
  and a request can name a Resource instead; the requests pages and notifications show
  the Resource when there is no service. Superseding for a Resource request goes by
  Resource + environment + subject. *(My call, from the review.)*
- For a queue, "attached" means its publishers other than its own consumer. The consumer
  is part of the queue's definition and does not block delete. *(My call; see section
  10.)* The existing "queue still holds messages / force" check stays as well.
- Deleting a **service** detaches it from every Resource; it never drops a database.
- Every create, attach, detach, adopt, backup download and delete writes an audit event
  without secret values.

## 7. Adoption (backwards compatibility)

Platform admins only. *(My call: adopting means pointing the platform at an arbitrary
schema, which is an admin-level act.)* Input: an existing schema name, the Resource name,
environment, owning team, and the services to attach.

The platform:

1. checks the schema exists and is not reserved or already managed;
2. creates a new scoped `kz_…` user with grants on that schema only;
3. records the database with `adopted = true` and attaches the services.

It never alters, renames or locks the schema and never touches the old user. Running
containers keep their old credentials until redeployed.

**Deleting an adopted database releases it**: the platform drops its `kz_…` user and its
record and leaves the schema and its data alone. The UI labels the action "Release"
instead of "Delete" and says the data stays; the API and MCP response say the same. The
attached-services block applies as for any delete.

Adoption is only finished when the apps read `PLATFORM_DB_*`:

- go-plateful (`api`, `worker`, `importer`) and legendsgrid change their config from
  `APP_MYSQL_*` to `PLATFORM_DB_*`, and their hand-set `APP_MYSQL_*` values and secrets
  are removed after the switched version is deployed in each environment.
- The gaardrail Go service template and the `creating-plateful-projects` and `kalzone`
  skills use `PLATFORM_DB_*` from then on.

Dropping the old shared user (`plateful`) is a separate, manual step for the admin once
every app has moved.

First targets: Plateful's `plateful` schema in prod (and its dev counterpart, if any),
attached to `api`, `worker`, `importer`; LegendsGrid's database. The builder checks the
live MySQL for the actual schema names before adopting.

## 8. Backup download

`GET` on the database returns a gzip-compressed SQL dump of that one schema
(`<schema>-<timestamp>.sql.gz`), streamed, never stored. It uses
`--single-transaction` semantics so it does not lock tables. It is available any time.
*(My calls, from the review:)* in `dev`, anyone who may view the database can download;
in a protected environment it needs a team maintainer (or platform admin), no change
request, and a token with a write-capable scope, so read-only agent tokens cannot pull
prod data. The UI shows a **Download backup** button on the database page and offers it
again in the delete confirmation. MCP gets a tool that returns a short-lived download
URL rather than the dump itself, so an agent does not stream a large file through tool
output; the URL still requires a signed-in session or a token, it is not a bearer link.

How the dump is produced (the `mysqldump` binary in the control plane image, or a Go
implementation) is left to the builder.

## 9. Surfaces

**UI**: the nav item **Queues** becomes **Resources**. The list shows every Resource
with kind, name, environment, team and attached services, filterable by kind and
environment. `New resource` asks for the kind first and then shows that kind's form.
Queue detail pages keep everything they have; existing `/queues/...` URLs redirect.
The database page shows the connection facts (host, port, schema, user; never the
password), the **Attached services** list with a detach action per row, attach, download
backup, and delete. The service page shows the Resources attached to it. Mockups come
first, per the standing rule for visual changes.

**API**: database endpoints next to the queue ones under `/api/v1`, plus a combined
`GET /api/v1/resources` listing. Errors carry hints, as everywhere else.

**MCP**: `listResources`, `createDatabase`, `getDatabase`, `attachDatabase`,
`detachDatabase`, `deleteDatabase`, `getDatabaseBackup`, `adoptDatabase`. Queue tools
stay as they are.

**Docs**: the `kalzone-skill` references (`new-service.md`, `platform-facts.md`) replace
"ask the admin for a database" with the self-service flow and list the `PLATFORM_DB_*`
names.

**Infra**: the control plane needs a MySQL account allowed to create and drop schemas
and users and grant on them (`CREATE`, `DROP`, `CREATE USER`, `GRANT OPTION` on `*.*`,
plus what the dump needs). It is a new account with its password from tfvars, not root
and not the `platform` user. *(My call.)*

## 10. Open for the builder or the review

- Section 4 limits a service to one database per environment. If an app ever needs two,
  the variable names need a per-database prefix.
- Section 6 decides that a queue's consumer does not block delete. The user's rule was
  that attached services block delete; the consumer is the exception because a queue
  cannot exist without one.

## 11. Order of delivery

Each step ships on its own:

1. **Resources section**: nav, combined list, `/queues` redirects, the queue publishers
   delete rule.
2. **Databases**: storage, MySQL provisioner, create/attach/detach/delete, deploy-time
   injection, reserved names, API, MCP, UI, infra account.
3. **Adoption and release**, then adopting Plateful and LegendsGrid on the live
   platform, with the app PRs that switch them to `PLATFORM_DB_*`.
4. **Backup download**, plus the delete confirmation offering it.
5. **Skill docs and Go template update.**

## 12. Left to the builder

Package layout inside the `gaardrail` structure, migration numbering, how the
provisioner talks to MySQL, dump implementation, exact UI layout within the mockups the
user approves, and test strategy (integration tests against the test MySQL in
`internal/testdb` are expected for grants and isolation).

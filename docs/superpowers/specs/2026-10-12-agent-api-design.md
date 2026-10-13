# Agent-grade API — design

Date: 2026-10-12
Status: approved in conversation (user accepted the recommended decisions on 2026-10-12), open for review
Sub-project 3 of 7 of "a platform made for AI agents". Builds on sub-projects 1 and 2,
`docs/superpowers/specs/2026-10-12-agent-identity-design.md` and
`docs/superpowers/specs/2026-10-12-approvals-design.md`, which are live.

## 1. Why

Agents now call `/api/v1` with their own tokens. On 2026-10-12 one of them shipped a
change and hit every rough edge of the API:

- There is no list of routes or fields. The agent grepped Go source, and still missed
  that `build_workflow` cannot be set when a service is created.
- Shapes differ per endpoint: `POST /deployments` returns the deployment,
  `GET /deployments/:id` returns `{deployment, events}`, every list has its own key.
- Status values are undocumented, so the agent guessed when a build or deploy was done.
- Builds and deploys are asynchronous; the agent wrote `sleep` + `GET` loops.
- `GET /deployments/:id` returns up to 500 raw log lines, mostly docker layer
  progress, and the agent's tool output was truncated.
- Everything is a UUID. Deploying one commit's shared build to `api`, `worker` and
  `importer` meant listing releases per service and filtering by `commit_sha`.
- A config revision replaces the whole spec, so a partial spec fails validation.
- "Is `LOG_LEVEL` set anywhere?" needed SQL.
- Unknown JSON fields are ignored silently; `?status=bogus` returns an empty list;
  `GET /tokens` returns team ids where everything else uses slugs.

After this sub-project an agent reads one OpenAPI document, gets the same shapes
everywhere, is told how to fix a bad call, waits for builds and deploys with one
call, names releases the way humans do, and ships a shared build in one request.

## 2. Decisions

The user's decisions of 2026-10-12, binding:

| # | Topic | Decision |
|---|---|---|
| 1 | Versioning | Change `/api/v1` in place, no `v2`. |
| 2 | Discovery | OpenAPI 3.1 generated from code. Every `Register*Routes` declares its operations with metadata (operationId, summary, request and response types, required scope, enums). Served at `/api/v1/openapi.json`. A test checks parity between `engine.Routes()` and the declared operations; the integration client validates every response against the document. The same metadata feeds the MCP tools of sub-project 4. |
| 3 | Shapes | A single resource is the bare object, also for actions (`202` + object). Lists are `{"items": [...], "next_cursor": "..."}`. `GET /deployments/:id` returns the deployment with an embedded `events_summary`. |
| 4 | Errors | `{"error": {"code", "message", "hint", "field", "details"}}`; hints tell the agent how to fix the call. Strict JSON: an unknown field is a `400` whose hint lists the accepted fields. |
| 5 | Pagination | `?limit` (default 20, max 100) and an opaque `?cursor`. |
| 6 | Waiting | `?wait=<duration>` (max 120 s) on async POSTs and on `GET` of a release, deployment or request. The server polls the database about once a second until a final status or the timeout: `200` when final, `202` + `Retry-After` when not. |
| 7 | Release references | A release is named by UUID, `latest` (last built), `r42`, `sha:<prefix>` or a tag, resolved per service. |
| 8 | Batch | `POST /deployments` accepts `{"targets": [{service, environment}], "release": "sha:abc123"}`: validate every target, then create all deployments in one transaction, all or nothing. Same for promotions. |
| 9 | Config patch | `PATCH /config/:service/:env` with `application/merge-patch+json` (RFC 7396) over `{spec, env, message}`; `null` removes. `POST …/revisions` stays. |
| 10 | Logs | `events_summary {total, phases, last 20 lines with docker progress collapsed}`; the full log at `GET /deployments/:id/events?cursor=&tail=`. |
| 11 | Config search | `GET /config/search?key=LOG_LEVEL[&env=][&team=]` over current revisions, including secret **names**, within the caller's teams. |
| 12 | Fixes | `GET /tokens` returns team slugs; `?status=bogus` is a `400`; `docs/onboarding-plateful.md` is rewritten to point at the OpenAPI document. |

Also binding: the build callbacks and `/agent/v1` keep their contracts; no SDK
generation, no `v2`.

Resolved in this document (section 19 lists the ones worth revisiting):

- The work ships in two plans: **3a** (the contract, every breaking change at once)
  and **3b** (waiting, release references, batch, config patch, config search — all
  additive). Section 3.
- swaggest/openapi-go generates the document, kin-openapi validates responses in
  tests. Section 5.
- A batch with a target that needs approval is refused as a whole. Section 15.
- `phases` are the deployment's waiting and running times, not parsed from the log.
  Section 10.
- The body field `release_id` of `POST /deployments` becomes `release` in 3a, so 3b
  only widens what it accepts.

## 3. Two plans

| Plan | Content | Breaks agents? |
|---|---|---|
| **3a — one contract** | operation registry, OpenAPI document and parity test, response validation in the integration client, list envelopes and cursors on every list, bare objects, errors with hints, strict JSON, `events_summary` and the events endpoint, token team slugs, status validation, `release_id` → `release`, onboarding doc | yes, once (section 7) |
| **3b — capabilities** | `?wait`, release references, batch deploy and promote, config merge patch, config search | no, everything is opt-in or new |

3a is planned in `docs/superpowers/plans/2026-10-12-agent-api.md`. 3b gets its own
plan after 3a is live; it declares its new operations through the registry 3a builds.
Agents break once, when 3a deploys, and the onboarding doc changes in the same
release.

## 4. Operation registry

Every `/api/v1` route is an `apispec.Operation`:

```go
type Operation struct {
	ID       string
	Method   string
	Path     string
	Summary  string
	Scope    scope.Scope
	Session  bool
	Gated    bool
	Params   any
	Body     any
	Status   int
	Response any
	Handler  gin.HandlerFunc
}
```

| Field | Meaning |
|---|---|
| `ID` | operationId, lowerCamel verb + noun: `listReleases`, `createDeployment`. Also the future MCP tool name. |
| `Path` | gin syntax relative to `/api/v1`: `/deployments/:id` |
| `Summary` | one sentence for humans and agents |
| `Scope` | the token scope the call needs (`read` for every `GET` except the read-free ones); empty for `whoami`, `me`, `tokens` and the document itself |
| `Session` | only a browser session may call it (`PUT /users/:login/admin`) |
| `Gated` | may answer `202` with a change request on a protected environment |
| `Params` | struct with `path:"…"` and `query:"…"` tags, documentation only; handlers keep reading `c.Param`/`c.Query` |
| `Body` | the request DTO; also what strict binding decodes into |
| `Status`, `Response` | success status and body type; `Response` is nil for `204` |

`httpserver.API` stops embedding `*gin.RouterGroup`. Its only method is
`Handle(ops ...apispec.Operation)`, which registers the handler on the guarded
`/api/v1` group and adds the operation to the `apispec.Catalog` (provided by fx).
A handler package therefore cannot add an `/api/v1` route without declaring it:
`api.GET` no longer compiles.

**Parity test.** `Test_EveryAPIRouteIsDeclared` (integration) compares the set of
`METHOD /api/v1/...` routes in `engine.Routes()` with the catalog. The only allowed
difference is the three build callbacks `POST /api/v1/releases/:id/{started,built,failed}`,
which are registered on the public router with OIDC authentication and stay out of
the document. A route registered on `Public` or `Web` under `/api/v1` fails the test.

The scope in the metadata is documentation; enforcement stays in `authz` and the
bearer guard. Sub-project 4 reads the same catalog to build MCP tools.

## 5. OpenAPI document

**Libraries.** Go 1.26 (go.mod `go 1.26.0`).

| Need | Choice | Why |
|---|---|---|
| Generate 3.1 from Go types | `github.com/swaggest/openapi-go` v0.2.61 (`openapi31`) with `github.com/swaggest/jsonschema-go` v0.3.78 | reflection over the existing DTOs, `path`/`query`/`json` tags, `enum` tags, security requirements with scopes, schema interceptors; go.mod `go 1.18`; checked: builds and generates a valid 3.1 document under Go 1.26 |
| Validate responses in tests | `github.com/getkin/kin-openapi` v0.149.0 (`openapi3`, `openapi3filter`, `routers/legacy`) | loads and validates 3.1 documents (type arrays, JSON Schema 2020-12); checked: rejects an unknown enum value and an extra property against a swaggest-generated document |

**Document rules**, applied by interceptors so every DTO follows them without tags:

- A property is **required unless its json tag has `omitempty`**. Request DTOs mark
  optional fields with `omitempty`, which changes nothing when decoding.
- Objects built from structs have `additionalProperties: false`, so a response with
  an undeclared field fails validation.
- Arrays and maps are never `null`: handlers build lists with `make`, and the schema
  says `type: array` or `type: object`.
- Schema names are `<Package><Type>` without a `DTO` suffix (`ReleasesRelease`,
  `DeploymentsDeployment`; an `entities` package is named after its domain,
  `QueueDeadLetter`); a list envelope is `<Item>Page`.
- `servers: [{url: <base_url>}]`, full paths (`/api/v1/deployments/{id}`).
- Two security schemes: `token` (HTTP bearer) and `session` (cookie
  `platform_session` + `X-CSRF-Token` on writes). An operation lists
  `[{token: [scope]}, {session: []}]`, or only `session` when `Session` is set.
  OpenAPI 3.1 allows role names in a bearer requirement, so the scope is
  machine-readable.
- Every operation has a `default` response with the error body (section 8). Gated
  operations also document `202` with the change-request body of sub-project 2;
  when their own answer is already `202` (deploy, promote, config apply, rollback)
  they declare `apispec.OrChangeRequest[DeploymentDTO]`, a `oneOf` of both bodies,
  and the build refuses a gated `202` operation that does not. Their description
  says "on a protected environment the token also needs the protected scope;
  without bypass the call answers 202 with a change request".
- Enums come from `enum:"…"` tags on DTO fields: release `status`
  (`requested,building,built,failed`), `source`, `trigger`; deployment `status`
  (`pending,in_progress,succeeded,failed,cancelled`), `strategy`, `trigger`
  (`manual,promote,rollback,config,reconcile` — rollback is a trigger, not a status);
  request `status` and `kind`; member `role`; the `status` query of `GET /requests`.

**Serving.** `GET /api/v1/openapi.json` is itself a declared operation
(`getOpenAPI`), behind the API guard and read-free like `whoami`: any session or
valid token gets it. The document is built once when the control plane starts; a
build error (a bad operation) stops the start, so a broken declaration never ships.

**Integration client.** The suite builds the same document from the catalog, loads
it into kin-openapi and validates **every** `/api/v1` response the test client
receives (status, content type, body), failing the test on a mismatch. Routes not in
the document (the callbacks) are not validated.

## 6. Response shapes

| Kind | Shape |
|---|---|
| single resource, read or created | bare object, `200` or `201` |
| action that starts async work (`POST /releases`, deploy, promote, config apply, rollback) | bare object, `202` |
| gated change waiting for approval | change request, `202` (unchanged from sub-project 2) |
| list | `{"items": [...], "next_cursor": "…"}`, `next_cursor` present only when there is more |
| delete, revoke | `204`, no body |
| queue dead-letter replay and purge | `202`, no body (nothing to return) |

## 7. What changes for current agents (3a)

| Endpoint | Before | After |
|---|---|---|
| every list (`/services`, `/teams`, `/teams/:team/members`, `/users`, `/tokens`, `/environments`, `/releases`, `/deployments`, `/config/:s/:e/revisions`, `/config/:s/:e/secrets`, `/queues`, `/queues/:e/:n/versions`, `/queues/:e/:n/dead`, `/audit`, `/requests`) | `{"<name>": [...]}`, whole list or a fixed cap | `{"items": [...], "next_cursor"}`, 20 per page by default |
| `GET /deployments/:id` | `{"deployment": {...}, "events": [500 lines]}` | the deployment with `events_summary` |
| `POST /deployments` | body field `release_id` | body field `release` |
| `GET /tokens`, `POST /tokens` | `team_ids` | `teams` (slugs) |
| `GET /queues/:e/:n/stats` | `{"stats": {...}, "fresh": bool}` | the stats object with `fresh` inside |
| `GET /audit` | `?before=<time>`, `?limit` clamped | `?cursor`, `?limit` 1–100 |
| `GET /requests?status=` | unknown status → empty list | `400` |
| `GET /releases`, `GET /deployments` | missing `service`/`env` → `404` | `400` naming the missing query parameter |
| every request body | unknown fields ignored | `400 err_invalid_input` with `field` and a hint |
| every `?limit` | clamped silently | `400` outside 1–100 |
| every error | `{code, message}` (+ `required_scope`, `team`) | adds `hint`, `field`, `details` when known; existing fields stay |

Unchanged: the change-request body and `/api/v1/requests/:id`, `GET /whoami`,
`GET /me`, every single-resource object other than the ones above.

**Untouched contracts:** the build callbacks `POST /api/v1/releases/:id/{started,built,failed}`
(handler, DTO, lenient binding and response), `/agent/v1`, the queue gateway
endpoints, the GitHub webhook, and the web UI (it calls usecases directly). They keep
the lenient `httperror.BindJSON`.

## 8. Errors

```json
{"error": {
  "code": "err_invalid_input",
  "message": "unknown field \"release_id\"",
  "field": "release_id",
  "hint": "remove release_id; this body accepts: service, environment, release, strategy",
  "details": {"accepted": ["service", "environment", "release", "strategy"]}
}}
```

`apperror.AppError` gains `Hint`, `Field` (`omitempty`) and `Details map[string]any`
(`omitempty`), with copying helpers `WithHint`, `WithField`, `WithDetails` so shared
error values are never mutated. `required_scope` and `team` stay where they are.

**Strict JSON.** `/api/v1` handlers bind with `httperror.BindStrict`, which decodes
with `DisallowUnknownFields` and then runs gin's validator:

| Problem | `field` | Hint |
|---|---|---|
| empty body | — | `send a JSON object; this body accepts: …` |
| malformed JSON | — | `the body is not valid JSON near byte N` |
| unknown field | the field | `remove F; this body accepts: …` (top-level fields, `details.accepted`) |
| wrong type | the field | `F must be a <type>` |
| missing required field | json name | `send F` |

**Hints added in 3a** (others can be added any time; a hint is optional text):

| Error | Hint |
|---|---|
| `err_missing_scope` | `ask the token's owner for a token with the S scope` |
| `err_team_not_granted` | `this token is limited to other teams; ask for a token granted this team` |
| `err_approval_required` (queue pause, dead letters) | `use a bypass token, or ask a human to do it in the browser` |
| `err_unauthenticated` on `/api/v1` | `send Authorization: Bearer <token>, or sign in to the website` |
| service, environment, team, release, deployment, queue or change request not found | the list endpoint that shows valid values, e.g. `list services with GET /api/v1/services` |
| release not built | `wait until GET /api/v1/releases/<id> says built` |
| a deployment is running | `follow the running deployment with GET /api/v1/deployments?service=<slug>&env=<env>, then deploy again` |
| `?limit`, `?cursor`, `?status`, missing query parameter | what is accepted |

## 9. Pagination

- `?limit`: 1–100, default 20; anything else is `400` (`field: limit`).
- `?cursor`: opaque (base64url of a key), copied from `next_cursor`; a cursor that
  does not decode, or names an item that no longer exists in an in-memory list, is
  `400` with hint `start again without cursor`.
- Lists are newest-first where they have a number or time, otherwise in their
  current order (slug, login, name).

| List | Key | Where |
|---|---|---|
| releases, deployments, config revisions, queue versions | `number` | SQL keyset: `number < ?` |
| audit events | `at` + `id` | SQL keyset: `(at, id) < (?, ?)` |
| change requests | `created_at` + `id` | SQL keyset; order becomes `created_at DESC, id DESC` |
| deployment events | `id`, oldest first | SQL keyset: `id > ?` |
| services, teams, members, users, tokens, environments, queues, secrets, dead letters | slug / login / id / name | in memory over the bounded list |

`pkg/page` holds `Request{Limit, After}`, `Page[T]{Items, Next}`, `Parse`, `Cut`
(trims a `Limit+1` fetch) and `Window` (in-memory). Keyset lists paginate in the
usecase — releases and deployments `PageFor`, config `RevisionsFor`, audit
`PageFor`, requests `PageFor`, queues `VersionPageFor` take a `page.Request` and
return a `page.Page` — and replace the old `ListFor`/`HistoryFor`/`VersionsFor`/
`GetFor`; the web UI keeps its own unpaged calls. The JSON envelope is
`apispec.Page[T]`; handlers answer with `apispec.RespondPage` (keyset) or
`apispec.RespondWindow` (in memory), which parse `?limit`/`?cursor` and convert to
DTOs.

## 10. Deployment events

`GET /deployments/:id` adds:

```json
"events_summary": {
  "total": 312,
  "progress_lines": 240,
  "phases": [{"name": "waiting", "seconds": 4}, {"name": "running", "seconds": 71}],
  "tail": [{"at": "…", "message": "api: r14.c3 is live on green"}],
  "log": "/api/v1/deployments/<id>/events"
}
```

- `total`: every stored line (a `COUNT`).
- `tail`: the last 20 lines that are not docker layer progress, read from the newest
  500. A progress line starts, after optional spaces and symbols, with a 12-character
  hex layer id (`5f70bf18a086 Downloading [==>  ]`); `progress_lines` counts those
  skipped among the 500.
- `phases`: `waiting` from creation to start (or now), `running` from start to
  finish (or now); a phase that has not begun is absent. Parsing deployctl's own
  steps from the log would tie the API to another repository's log format.

`GET /deployments/:id/events` returns the full log, oldest first, as a list of
`{at, message}`:

- without parameters: from the first line; `?cursor` continues;
- `?tail=N` (1–100): the last N lines; not combined with `?cursor` (`400`);
- `next_cursor` is present when the page is full **or** the deployment is still
  active, so an agent can follow a running deploy by repeating the call with it.

Lists of deployments and action responses carry no summary.

## 11. Small fixes

- `GET /tokens` and `POST /tokens` return `teams` as slugs (`AccessToken` gains
  `TeamSlugs`, filled by the tokens usecase with the existing `SlugsByIDs`).
- `GET /requests?status=` accepts the seven statuses or nothing; anything else is
  `400` with the accepted values.
- `docs/onboarding-plateful.md` is rewritten: services and environments as they are
  now (worker and importer have dev environments), and "the API is described at
  `GET /api/v1/openapi.json`" instead of route lists.
- README: an "API for agents" section with shapes, errors, pagination and the
  document URL.

## 12. Waiting (3b)

`?wait=<Go duration>` (`30s`, `2m`), 0 < wait ≤ 120 s, otherwise `400`.

| Accepted on | Waits for | Final statuses |
|---|---|---|
| `GET /releases/:id`, `POST /releases` | the release | `built`, `failed` |
| `GET /deployments/:id`, `POST /deployments` (single and batch), promotions, config applications, rollback | the deployment(s) | `succeeded`, `failed`, `cancelled` |
| `GET /requests/:id`, any gated call that answered with a change request | the request | `executed`, `failed`, `rejected`, `cancelled`, `expired` |

- The answer is `200` with the object when final, else `202` with the object and
  `Retry-After: 5`. A batch is final when every deployment is.
- **How:** the handler re-reads through the same usecase (`GetFor`), every second,
  until final, deadline, or the client goes away. The database is the only source
  of truth, so during a blue-green switch either control-plane instance sees the same
  state, and a deployment finished through the other instance is seen within a
  second. The in-process event bus is not used for waiting.
- **Shutdown:** `httpserver.Run` registers `server.RegisterOnShutdown`, which closes a
  drain channel; every waiter returns its current state (`202`) at once, so the old
  instance's `Shutdown` finishes within fx's stop timeout instead of cutting waits.
- Traefik's `readTimeout` is 300 s and the platform is not behind Cloudflare, so a
  120 s response is safe.

## 13. Release references (3b)

Accepted in `release` of `POST /deployments` (single and batch) and in
`GET /releases/:id` when `?service=` is given:

| Form | Resolves to (within the service) |
|---|---|
| UUID | that release; must belong to the service |
| `latest` | the built release with the highest number |
| `r42` | release number 42, any status |
| `sha:<7–40 hex>` | the newest built release whose `commit_sha` starts with the prefix; two different commits matching → `400` "ambiguous, send more characters" |
| anything else | the newest built release with that tag |

The release domain resolves (`ReleasesUsecase.Resolve`, new repository queries
`LatestBuilt`, `ByNumber`, `BuiltByCommitPrefix`, `BuiltByTag`); deploy reaches it
through its `ReleaseLookup` adapter. A reference that matches nothing is `404` with
the hint `list releases with GET /api/v1/releases?service=S`. Change requests pin the
resolved release id, as today.

## 14. Config merge patch (3b)

`PATCH /api/v1/config/:service/:env`, `Content-Type: application/merge-patch+json`
(else `415` with a hint), body a merge patch over `{"spec": {...}, "env": {...}}`
plus an optional `"message"`:

```json
{"spec": {"replicas": 2}, "env": {"LOG_LEVEL": "debug", "OLD_FLAG": null}, "message": "more replicas"}
```

- The patch is applied (RFC 7396, a 20-line function, no library) to the current
  revision's `{spec, env}`; the merged spec is decoded strictly into `Spec`, so an
  unknown spec field is a `400`; `"spec": null` is a `400`.
- The result becomes the existing `ChangeSet` (full merged spec when `spec` was
  sent, `set_env` for changed keys, `unset_env` for nulls) and goes through `Revise`:
  same validation, same no-op check, same approval gate (`201` + revision or `202`
  change request).
- Secrets are not patchable here; they keep their own endpoints.

## 15. Batch deploy and promote (3b)

```json
POST /api/v1/deployments
{"targets": [{"service": "api", "environment": "prod"}, {"service": "worker", "environment": "prod"}],
 "release": "sha:abc1234", "strategy": "bluegreen"}

POST /api/v1/deployments/promotions
{"targets": [{"service": "api", "environment": "prod"}, {"service": "web", "environment": "prod"}], "from": "dev"}
```

- `targets` (1–10, no duplicates) replaces `service`+`environment` (or `to`); sending
  both forms is a `400`. The single forms stay.
- Every target is validated first exactly like a single call: service, environment,
  authorization and gate, release resolved **per service**, built, nothing running.
- **Gated targets:** if any target's gate is `needs_approval`, the whole batch is
  refused with `403 err_approval_required`, `field: targets[i]` and the hint "send
  the targets on protected environments one at a time; each becomes a change
  request". A request is about one service in one environment; a batch request that
  spans teams has no single approver, and half-approved batches are not all or
  nothing. Bypass tokens and browser sessions batch to prod directly.
- Any failure answers with the first error, `field: targets[i]`, and
  `details.targets` listing every failing target `{index, code, message}`; nothing is
  written.
- Then one repository call, `DeploymentRepository.ScheduleAll`, runs in a single
  transaction: for each target cancel its pending deployments (as `clearWay` does),
  re-check nothing is in progress, insert the deployment. The audit events and bypass
  events are published after the commit, one per deployment.
- The answer is `202` with `{"items": [deployments in target order]}`; the 202 schema
  becomes a `oneOf` of single deployment, page of deployments and change request
  (jsonschema-go `OneOfExposer`, like `apispec.OrChangeRequest`).

## 16. Config search (3b)

`GET /api/v1/config/search?key=LOG_LEVEL[&env=prod][&team=plateful]`, scope `read`:

```json
{"items": [
  {"service": "api", "environment": "prod", "revision": 12, "kind": "env", "value": "info"},
  {"service": "worker", "environment": "prod", "revision": 7, "kind": "secret"}
]}
```

- `key` is required and must be a valid variable name (it is also used in a JSON
  path); exact match.
- Searches the current revision of every service and environment: `env` keys with
  their value, `secrets` keys with no value, ever.
- Visibility: only services whose team the caller sees (`actor.Sees`); `team`
  narrows further, a team the token is not granted is `403 err_team_not_granted`.
- The envconfig repository finds `(service_id, environment, number, kind, value)`
  with `JSON_CONTAINS_PATH` over current revisions; service slugs and teams come
  through the envconfig catalog adapter, not a SQL join, following the "no shared
  SQL" rule of the microservices plan. Paginated in memory, ordered by service and
  environment.

## 17. Code layout

| Where | What | Plan |
|---|---|---|
| `pkg/apperror` | `Hint`, `Field`, `Details`, `WithHint`, `WithField`, `WithDetails` | 3a |
| `pkg/page` | requests, pages, cursors | 3a |
| `internal/apispec` | `Operation`, `Catalog`, `Page[T]`, `Items`, document builder | 3a |
| `internal/httpserver` | `API.Handle`, `getOpenAPI` | 3a |
| `internal/httperror` | `BindStrict` | 3a |
| repositories of release, deploy, envconfig, audit, approvals, queue | keyset queries, `EventsAfter`, `CountEvents` | 3a |
| usecases of the same domains, tokens | `page.Request` in, `page.Page` out, `Summarize`, slugs, status check | 3a |
| every `/api/v1` handler package | operations, envelopes, strict binding | 3a |
| `internal/integration` | document loaded in kin-openapi, every response validated, parity test | 3a |
| `internal/httpserver/wait`, handlers | waiter, drain on shutdown | 3b |
| release domain | `Resolve`, reference queries | 3b |
| deploy domain | batch validation, `ScheduleAll` | 3b |
| envconfig domain | merge patch, search | 3b |

## 18. Testing

3a:

- Unit, `pkg/page`: defaults and bounds of `limit`, cursor round trip and garbage,
  `Cut` with fewer, exactly and more than the limit, `Window` after a key and with a
  vanished key.
- Unit, `apperror`: helpers copy, JSON omits empty fields.
- Unit, `httperror.BindStrict`: unknown field with accepted list, missing required
  field by json name, wrong type, empty body, malformed JSON; valid body passes.
- Unit, `apispec`: a catalog builds a 3.1 document with required/optional fields,
  `additionalProperties: false`, non-null arrays, enums, security scopes, default
  error and gated 202; a bad path parameter fails the build.
- Unit, deploy: `Summarize` (progress lines skipped and counted, tail of 20, phases
  for pending, running and finished deployments); `EventsFor` with tail, cursor, and
  follow on an active deployment.
- Unit, repositories: each keyset query returns the next page without overlap.
- Unit, tokens: list and create carry team slugs. Requests: bogus status is `400`.
- Integration: `Test_EveryAPIRouteIsDeclared`; the document loads and validates;
  every existing test now runs with response validation; strict JSON answers with
  field and hint; paging through releases and the audit log; `GET /deployments/:id`
  summary; following a running deployment's events; tokens show slugs.

3b: waiting returns 200 on final and 202 + `Retry-After` on timeout and on drain;
a wait sees a status written through another connection; each reference form and
ambiguity; batch all-or-nothing on a running target, refusal on a gated target,
bypass batch to prod; merge patch partial spec, null removal, unknown spec field,
wrong content type, gated patch; search finds env values and secret names, hides
other teams' services and never returns a secret value.

## 19. Decisions to revisit

| Decision | Cost if wrong |
|---|---|
| A batch with any gated target is refused instead of becoming one change request | Agents without bypass send N single prod deploys and humans approve N requests. A `deploy.batch` request kind with one approver per team is a new approvals kind plus an executor. |
| `phases` are timing only | Agents still read the tail to see which deployctl step failed. Parsing deployctl markers is one function in `Summarize`. |
| Small lists paginate in memory | Fine up to thousands of rows; a list that grows (queues) moves to a keyset query, a repository change only. |
| Required unless `omitempty` | A field that is optional in a request but lacks `omitempty` is documented as required; the validation of responses catches the opposite. Fix by adding the tag. |
| Hints are free text | Agents must read them; a machine-readable `fix` object can be added under `details` later without breaking anyone. |
| The document is behind authentication | An agent without a token cannot read it; moving `getOpenAPI` to the public router is one line and one parity exception. |
| `release_id` renamed to `release` in 3a | One more breaking change in 3a; keeping both names would have been permanent ambiguity. |

## 20. Done when

**3a:** an agent with a fresh token fetches `/api/v1/openapi.json`, finds
`createDeployment` with its body, scope and statuses, lists `api`'s releases with
`limit=5` and follows `next_cursor`, deploys with `{"service", "environment",
"release"}`, gets the deployment back as a bare object, reads `GET
/deployments/:id` with a 20-line `events_summary` instead of 500 lines, sends a body
with a misspelled field and is told which fields exist; `go test ./...` validates
every API response of the integration suite against the document, and a route added
without a declaration fails the build or the parity test.

**3b:** the same agent runs `POST /api/v1/deployments?wait=120s` with
`{"targets": [api, worker, importer in dev], "release": "sha:abc1234"}` and gets
`200` with three succeeded deployments, patches `{"spec": {"replicas": 2}}`, and
answers "is `LOG_LEVEL` set anywhere?" with one `GET /config/search?key=LOG_LEVEL`.

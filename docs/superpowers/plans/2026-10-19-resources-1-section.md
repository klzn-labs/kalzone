# Resources 1 — Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A **Resources** section in the UI, the API and MCP that lists every managed resource (queues today), with the shared rule that a resource with attached services cannot be deleted.

**Architecture:** A thin `app/resources` module aggregates resources from per-kind `Source` ports; queues are its first source. Queue storage and the queue domain stay as they are, apart from the new delete rule. The web queue pages move under `/resources/queues/...`, and the old `/queues...` URLs answer with permanent redirects.

**Tech Stack:** Go 1.26, gin, gorm/MySQL, Uber fx, htmx templates, testify.

**Spec:** `docs/superpowers/specs/2026-10-19-resources-design.md` (delivery step 1)

## Global Constraints

- No comments in any file (Go, SQL, HTML, tests). Functions ≤ 20 lines, ≤ 2 levels of nesting.
- Queue storage, the gateway, the SDK and the `PLATFORM_QUEUE_*` names do not change.
- "Attached" for a queue = its publishers other than its own consumer. The consumer never blocks delete.
- The existing "queue still holds messages / force" check stays, and runs after the attached check. `force` never bypasses the attached check.
- Visual changes: show mockup options as an artifact and get the user's pick before writing templates.
- One commit per layer at the end (rules → handlers+wiring+integration), via the layered-commits skill.

## Review Focus

- A queue whose only publisher is its consumer: delete goes through (no false block). Test in Task 1.
- A prod queue with publishers: the delete is refused **before** a change request is filed, so nobody approves a request that can only fail. Test in Task 1.
- A bookmarked `/queues/dev/x?tab=dead` keeps its query string through the redirect. Test in Task 3.
- An agent token with only `read` gets `listResources` and the list is filtered to its teams. Test in Task 2.
- A publisher removed by service deletion unblocks the delete straight away (the gateway checks publishers live, so no drain window exists for queues). Test in Task 1.

## Deviation from the spec

Spec §6 says the "detached but still running" rule "applies to queue publishers" too. It needs no code for queues: publish rights are checked live by the gateway against `queue_publishers`, so a removed publisher loses access immediately and nothing keeps running on old rights.

---

### Task 1: Queue delete refused while publishers are attached

**Files:**
- Modify: `app/queue/usecases/queues/delete.go`
- Test: `app/queue/usecases/queues/delete_test.go` (create if absent; reuse the fakes in `fakes_test.go`)

**Interfaces:**
- Produces: `(entities.Queue) Attached() []string`, the publisher IDs without the consumer, in `app/queue/entities/queue.go`. The error is `apperror.Conflict("the queue still has attached services")` with hint `"remove these publishers first: <slug>, <slug>"` (slugs via `u.services.FindServiceByID`, in publisher order).

- [ ] **Step 1: Failing tests.**
  - `Test_Delete_RefusesWhilePublishersAreAttached`: queue with consumer `w` and publishers `[w, api]` → `apperror.CodeConflict`, hint contains `api` and not `w`; store `Delete` not called.
  - `Test_Delete_ConsumerAloneDoesNotBlock`: publishers `[w]`, stats fresh and empty → nil.
  - `Test_Delete_ForceDoesNotBypassAttached`: publishers `[w, api]`, `force=true` → conflict.
  - `Test_Delete_AttachedCheckedBeforeApproval`: protected env (gate `NeedsApproval`), publishers `[w, api]` → conflict; `requests.Submit` not called.
  - `Test_Delete_AllowedOnceThePublisherIsDropped`: run `Detach(ctx, a, apiID)`, then `Delete` → nil.
- [ ] **Step 2: Run** `go test -p 1 ./app/queue/...` → the new tests FAIL.
- [ ] **Step 3: Implement** `Attached()` and an `attachedError(ctx, q) error` used in `Delete` right after `editable`.
- [ ] **Step 4: Run** `go test -p 1 ./app/queue/...` → PASS.
- [ ] **Step 5: Commit** `git commit -m "Refuse deleting a queue that services still publish to"`

### Task 2: `app/resources` and `GET /api/v1/resources`

**Files:**
- Create: `app/resources/entities/resource.go`
- Create: `app/resources/usecases/resources/resources.go`, `resources_test.go`
- Create: `app/resources/repositories/queue/queue.go`
- Create: `app/resources/handlers/resources/resources.go`, `dto.go`, `operations.go`
- Create: `cmd/controlplane/modules/resources.go`; register in the controlplane fx app next to `Queue()`
- Test: `internal/integration/tests/resources_test.go`

**Interfaces:**
- Produces:
  - `entities.Kind` consts `KindQueue = "queue"`, `KindDatabase = "database"`.
  - `entities.Resource{Kind Kind; Name, Environment, TeamID string; Attached []string; CreatedAt time.Time}`; `Attached` holds service slugs.
  - `resources.Source interface { Resources(ctx context.Context, a actor.Actor, environment string) ([]entities.Resource, error) }`
  - `resources.Sources map[entities.Kind]Source`
  - `resources.NewResourcesUsecase(sources Sources) ResourcesUsecase`
  - `(u ResourcesUsecase) ListFor(ctx, a actor.Actor, kind entities.Kind, environment string) ([]entities.Resource, error)`: an empty kind means all kinds; an unknown kind is `apperror.Invalid` with field `kind` and hint `send one of queue, database`. Sorted by environment, then kind, then name.
  - `queue.NewSource(queues Queues, names ServiceNames) Source`: wraps `QueuesUsecase.ListFor` (already team-filtered); `Attached` = slugs of `q.Attached()`.
  - Operation `listResources`: `GET /resources`, scope `read`, query `kind`, `environment` + `apispec.ListParams`, response `apispec.Page[ResourceDTO]` with `ResourceDTO{Kind, Name, Environment, TeamID, Attached []string, CreatedAt}` (json snake_case), key `kind/environment/name`. MCP picks it up automatically.

- [ ] **Step 1: Failing unit tests.** `Test_ListFor_MergesSourcesSorted`, `Test_ListFor_OnlyTheAskedKind`, `Test_ListFor_RejectsUnknownKind`.
- [ ] **Step 2: Failing integration test.** `Test_ResourcesListQueuesWithAttachedServices` (with `setupQueue`): create `place-import` in dev; `GET /api/v1/resources` as bob → one item `{kind: queue, name: place-import, attached: [places-api]}`; as eve (not in team) → empty; `?kind=nope` → 400. `Test_ResourcesToolIsListedForReadTokens`: MCP `tools/list` with a `read`-only token contains `listResources`.
- [ ] **Step 3: Run** `go test -p 1 ./app/resources/... ./internal/integration/...` → FAIL.
- [ ] **Step 4: Implement** the entities, the usecase, the queue source, the handler and the fx module.
- [ ] **Step 5: Run** the same tests → PASS. Run `go test -p 1 ./internal/integration/tests -run Contract` → PASS (the OpenAPI contract test covers the new operation).
- [ ] **Step 6: Commit** `git commit -m "List every managed resource at /api/v1/resources"`

### Task 3: Resources pages (mockups first)

**Files:**
- Create: `app/web/handlers/resources/resources.go` (list + new-kind picker + redirects)
- Create: `app/web/templates/pages/resources.html`, `resource_new.html`
- Modify: `app/web/handlers/queues/queues.go` (routes under `/resources/queues`), `actions.go` (redirect targets)
- Modify: `app/web/templates/layout.html` (nav), `pages/queue.html`, `pages/queue_new.html`, `pages/queues.html` (remove; folded into `resources.html`)
- Modify: `cmd/controlplane/modules/web.go`
- Test: `internal/integration/tests/queue_pages_test.go`, `internal/integration/tests/resource_pages_test.go`

**Interfaces:**
- Consumes: `resources.ResourcesUsecase.ListFor` (Task 2); `entities.Queue.Attached()` (Task 1).
- Produces: routes `GET /resources?kind=&environment=`, `GET /resources/new`, `GET /resources/new/queue` (the old create form), `/resources/queues/:env/:name[...]` (all former queue routes), and `GET /queues`, `/queues/new`, `/queues/*rest` → `301` to the matching `/resources...` URL with the query string kept. Nav key `resources`, label **Resources**, icon `layers`.

- [ ] **Step 1: Mockups.** Publish an artifact with 2–3 options for: the Resources list (kind + environment filters; columns kind, name, env, team, attached services; queue stats inline or not), the "New resource" kind picker, and the queue page's delete panel when blocked (disabled button + "Remove these publishers first: …"). Wait for the user's pick and record it in the PR description.
- [ ] **Step 2: Failing tests.** `Test_ResourcesPageListsQueues` (`/resources` shows `place-import` and `places-api` in the attached column; nav item `Resources` active), `Test_OldQueueURLsRedirect` (`/queues` → `/resources?kind=queue`; `/queues/dev/place-import?tab=dead` → `/resources/queues/dev/place-import?tab=dead`, status 301), `Test_QueueDeleteBlockedShowsWhy` (queue page with an extra publisher renders the disabled delete button and `places-api`). Update the existing `queue_pages_test.go` paths to `/resources/queues/...`.
- [ ] **Step 3: Run** `go test -p 1 ./internal/integration/... -run 'Resource|Queue'` → FAIL.
- [ ] **Step 4: Implement** the handlers and templates following the chosen mockup.
- [ ] **Step 5: Run** `go test -p 1 ./...` → PASS. Run `make lint` → clean.
- [ ] **Step 6: Check in a browser.** `make run`: the list, the filters, the redirect and the blocked delete, in light and dark.
- [ ] **Step 7: Commit** `git commit -m "Show queues under a Resources section"`

### Task 4: Ship

- [ ] **Step 1:** Rewrite the branch with the layered-commits skill (rules → handlers+wiring+integration).
- [ ] **Step 2:** Push, open the PR (spec + this plan linked, mockup choice in the description), merge with rebase, release and deploy the control plane following the standing deploy procedure; check `/resources` on `platform.kalz.one`.

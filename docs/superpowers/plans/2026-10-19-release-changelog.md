# Release Changelog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Freeze a GitHub changelog on every built release and show, on promotion requests, the commits that ship, in the UI, the API and MCP.

**Architecture:** The release domain owns changelogs: a `release_changelogs` table, a GitHub compare adapter, and a `changelogs` usecase that fills rows in the background after `release.built`/`release.shared` and retries them from a sweeper. It also answers `Between(from, to)`, which the deploy domain calls through a port when it files a change request, freezing the result inside the request's proposal payload.

**Tech Stack:** Go 1.26, gin, gorm/MySQL, Uber fx, htmx templates, testify, `internal/fakegithub`.

**Spec:** `docs/superpowers/specs/2026-10-19-release-changelog-design.md`

## Global Constraints

- No comments in any file (Go, SQL, HTML, tests). Functions ≤ 20 lines, ≤ 2 levels of nesting.
- No new GitHub App permission: only `GET /repos/{repo}/compare/{base}...{head}` and `GET /repos/{repo}/commits/{sha}/pulls`.
- At most 30 PR lookups per changelog; compare responses over 250 commits set `truncated`.
- Retry: `pending` rows retried when `updated_at` is older than `2^attempts` minutes; after 5 attempts → `failed`.
- States: `pending`, `ahead`, `first`, `diverged`, `unavailable`, `failed`.
- The changelog subscriber never blocks the build callback: it writes the `pending` row and returns.
- One commit per layer at the end (storage → rules → handlers+wiring+integration), via the layered-commits skill.

## Review Focus

- A release whose base finished out of order (r8 done before r7): r8 compares with r6, r7 with r8. Test in Task 3.
- Promote over a feature-branch release in between: grouping drops commits not in the compare, and counts distinct SHAs. Test in Task 4.
- GitHub down during request creation: the request is still created, `ships.state = failed`. Test in Task 6.
- Registered release (no SHA) as candidate or live: `unavailable`, no GitHub call. Test in Task 4.
- Executed request: `getRequest` still returns `preview.ships`, without `lines`. Test in Task 7.

## Deviation from the spec

The spec says a new `change_requests.ships` column. The plan freezes `ships` inside the request's existing `payload` (the deploy `Proposal` JSON) instead: same behaviour (frozen at creation, returned for every status), no approvals migration, and the approvals domain stays unaware of deploy data. Dev deploys need no request, so they get no `ships` (the spec's "computed but not stored" has no screen to show it on).

---

### Task 1: Storage — `release_changelogs` and changelog entities

**Files:**
- Create: `migrations/000010_release_changelogs.up.sql`, `.down.sql`
- Create: `app/release/entities/changelog.go`, `app/release/entities/changelog_test.go`
- Create: `app/release/repositories/mysql/changelogs.go`, `changelogs_test.go`
- Modify: `app/release/repositories/mysql/releases.go` (add `PreviousBuilt`)

**Interfaces:**
- Produces:
  - `entities.ChangelogState` consts `ChangelogPending|Ahead|First|Diverged|Unavailable|Failed`.
  - `entities.Changelog{ReleaseID, BaseReleaseID, BaseSHA, HeadSHA string; State ChangelogState; AheadBy, BehindBy int; Truncated bool; CompareURL string; Commits []Commit; Attempts int; LastError string; UpdatedAt time.Time}`
  - `entities.Commit{SHA, Title, Type, Author string; CommittedAt time.Time; PullRequest *PullRef}`, `entities.PullRef{Number int; Title, URL string}`
  - `entities.ParseCommit(sha, message, author string, at time.Time) Commit` (title = first line, type = conventional prefix before `(`/`!`/`:` lower-cased if in `feat fix chore docs refactor perf test build ci style revert`, else `other`)
  - `(c Changelog) RetryDue(now time.Time) bool`
  - `mysql.ChangelogRepository`: `Save(ctx, c) error` (upsert by release_id), `Find(ctx, releaseID) (Changelog, bool, error)`, `FindMany(ctx, ids []string) (map[string]Changelog, error)`, `Pending(ctx) ([]Changelog, error)`, `ByPair(ctx, baseSHA, headSHA string) (Changelog, bool, error)` (only non-pending, non-failed rows)
  - `ReleaseRepository.PreviousBuilt(ctx, serviceID, excludeID string, finishedBefore time.Time) (Release, bool, error)`: newest by `finished_at` with `status=built`, `commit_sha<>''`, `finished_at < ?`, `id <> ?`.

- [ ] **Step 1: Migration.** Table `release_changelogs`: `release_id CHAR(36) PK, base_release_id CHAR(36) NOT NULL DEFAULT '', base_sha CHAR(40) NOT NULL DEFAULT '', head_sha CHAR(40) NOT NULL DEFAULT '', state VARCHAR(16) NOT NULL, ahead_by INT NOT NULL DEFAULT 0, behind_by INT NOT NULL DEFAULT 0, truncated BOOL NOT NULL DEFAULT FALSE, compare_url VARCHAR(512) NOT NULL DEFAULT '', commits JSON NOT NULL, attempts INT NOT NULL DEFAULT 0, last_error VARCHAR(512) NOT NULL DEFAULT '', updated_at DATETIME(3) NOT NULL, KEY release_changelogs_state (state, updated_at), KEY release_changelogs_pair (base_sha, head_sha)`. Also `ALTER TABLE releases ADD KEY releases_finished (service_id, finished_at)`. Down drops both.
- [ ] **Step 2: Failing tests.** `Test_ParseCommit_ReadsConventionalType` (`"feat(lists): share\n\nbody"` → Title `feat(lists): share`, Type `feat`; `"Fix: x"` → `fix`; `"tidy logs"` → `other`; `"feat!: drop v1"` → `feat`). `Test_RetryDue_BacksOffExponentially` (attempts 2, updated 3m ago → false; 5m ago → true). Repo: `Test_Save_RoundTripsCommits` (PullRequest pointer kept, nil kept), `Test_Save_Upserts`, `Test_Pending_OnlyPendingRows`, `Test_ByPair_IgnoresPendingAndFailed`, `Test_PreviousBuilt_ByFinishTimeNotNumber` (r1 finished t1, r3 finished t2, r2 finished t3 → PreviousBuilt for r2 with finishedBefore t3 returns r3; skips registered release without SHA).
- [ ] **Step 3: Run** `go test -p 1 ./app/release/...` → FAIL (undefined).
- [ ] **Step 4: Implement** entities, gorm model with `commits` as `serializer:json`, repository methods, `PreviousBuilt`.
- [ ] **Step 5: Run** `go test -p 1 ./app/release/... ./migrations/...` → PASS.
- [ ] **Step 6: Commit** `git commit -m "Store release changelogs"`

### Task 2: GitHub adapter — compare and PR lookup

**Files:**
- Create: `app/release/repositories/githubapp/compare.go`, `compare_test.go`
- Modify: `internal/fakegithub/app.go` (routes + setters)

**Interfaces:**
- Produces:
  - `entities.Comparison{Status string; AheadBy, BehindBy int; Commits []Commit; URL string; Truncated bool}` (in `app/release/entities/changelog.go`), commits newest first.
  - `(c Client) Compare(ctx, repo, base, head string) (entities.Comparison, error)`; `Truncated` when `total_commits > len(commits)`; `Author` = `author.login` else `commit.author.name`; `URL` = `html_url`.
  - `(c Client) PullFor(ctx, repo, sha string) (*entities.PullRef, error)`: first merged PR from `/commits/{sha}/pulls`, nil when none.
  - `(c Client) Commit(ctx, repo, sha string) (entities.Commit, error)`: `GET /repos/{repo}/commits/{sha}`.
  - fakegithub: `(s *Server) WillCompare(repo, base, head string, c fakegithub.Comparison)`, `WillPull(repo, sha string, number int, title string)`, `FailCompare(repo string)`; `fakegithub.Comparison{Status string; AheadBy, BehindBy, Total int; Commits []fakegithub.Commit}`, `fakegithub.Commit{SHA, Message, Login string}`; `CompareCalls() int`.

- [ ] **Step 1: Failing tests.** `Test_Compare_ReadsCommitsNewestFirst` (fake returns oldest-first like GitHub; client returns newest first, Type parsed, URL set), `Test_Compare_FlagsTruncation` (Total 300, 250 commits → Truncated), `Test_PullFor_ReturnsNilWithoutPR`, `Test_Compare_ErrorNamesTheRepo` (FailCompare → error containing `o/api`).
- [ ] **Step 2: Run** `go test ./app/release/repositories/githubapp/` → FAIL.
- [ ] **Step 3: Implement** via existing `installed(...)` helper; base/head path-escaped.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `git commit -m "Compare commits through the GitHub App"`

### Task 3: Rules — changelogs usecase (fill, retry)

**Files:**
- Create: `app/release/usecases/changelogs/changelogs.go`, `fill.go`, `sweep.go`, `changelogs_test.go`, `fakes_test.go`

**Interfaces:**
- Consumes: Task 1 store/entities, Task 2 `Compare`/`PullFor`.
- Produces:
  - Ports: `Store` (Task 1 changelog methods), `Releases{FindByID; PreviousBuilt}`, `Services{FindServiceByID(ctx, id) (entities.ServiceRef, error)}`, `GitHub{Compare; PullFor}`.
  - `NewChangelogsUsecase(store Store, releases Releases, services Services, github GitHub) ChangelogsUsecase`; field `run func(func())` defaults to `func(f func()){ go f() }`; test helper `WithRunner(run func(func())) ChangelogsUsecase`.
  - `Handle(ctx, e events.Event) error`: only `release.built`/`release.shared` with `TargetKind == release`; saves `pending` row (HeadSHA from release) then `run(func(){ u.Fill(context.WithoutCancel(ctx), id) })`. Release without SHA → nothing saved.
  - `Fill(ctx, releaseID string) error`: base via `PreviousBuilt(serviceID, id, *FinishedAt)`; none → `first` with the head commit only, read with `GitHub.Commit(ctx, repo, sha)`; on error the normal retry path applies. Reuse `ByPair` when found. Else Compare → state from status (`ahead`/`identical` → `ahead`, `behind`/`diverged` → `diverged`), PR lookups for the first 30 commits. Error → `Attempts++`, `LastError` (clipped 512), still `pending`; at 5 → `failed`.
  - `Sweep(ctx) (int, error)`: `Fill` every `Pending` row with `RetryDue(now)`, returns how many.
  - `Get(ctx, rel entities.Release) (entities.Changelog, error)`: release without SHA → `{State: unavailable}`; no row → `{State: pending}`.
  - `Many(ctx, ids []string) (map[string]entities.Changelog, error)`.

- [ ] **Step 1: Failing tests.** `Test_Handle_SavesPendingAndFillsInBackground` (sync runner; row ends `ahead` with 2 commits, PR attached), `Test_Handle_IgnoresOtherEvents`, `Test_Fill_FirstRelease`, `Test_Fill_OutOfOrderFinishUsesFinishTime` (from Review Focus), `Test_Fill_ReusesSamePair` (CompareCalls stays 1 for a shared sibling), `Test_Fill_DivergedKeepsCounts`, `Test_Fill_CapsPullLookupsAt30` (40 commits → 30 PullFor calls), `Test_Fill_FailsAfterFiveAttempts`, `Test_Sweep_RetriesOnlyDueRows`, `Test_Get_RegisteredIsUnavailable`.
- [ ] **Step 2: Run** `go test ./app/release/usecases/changelogs/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `git commit -m "Fill release changelogs in the background"`

### Task 4: Rules — what ships between two releases

**Files:**
- Create: `app/release/usecases/changelogs/between.go`, `between_test.go`
- Modify: `pkg/approval/approval.go` (add `Ships`)

**Interfaces:**
- Produces:
  - `approval.Ships{State, Direction string; From, To int; Releases []ShipRelease; Unattributed []ShipCommit; CommitCount, PullCount int; Truncated bool; CompareURL, Error string}` with snake_case JSON tags; `approval.ShipRelease{Number int; State string; Commits []ShipCommit}`; `approval.ShipCommit{SHA, Title, Type, Author string; PullNumber int; PullTitle, PullURL string}`; `approval.Preview` gains `Ships *Ships`.
  - `(u ChangelogsUsecase) Between(ctx, fromReleaseID, toReleaseID string) (approval.Ships, error)`: `fromReleaseID == ""` → `State first`, the candidate's own changelog as one release. Either side without SHA → `unavailable`. Else one `Compare(repo, from.sha, to.sha)`: `ahead` → `Direction forward`, `behind` → `backward` (compare reversed: `Compare(to, from)`, commits are what leaves), `diverged` → `forward` + `State diverged`. Group: list releases strictly between the two numbers (exclusive low, inclusive high, via `Releases.ListByService(serviceID, high+1, high-low)`, add to the `Releases` port), load their changelogs with `Many`, attribute each commit to the lowest-numbered release whose changelog has its SHA, else `Unattributed`; PR info from that changelog. Counts are distinct SHAs / distinct PR numbers. GitHub error → `State failed`, `Error` message, nil error returned.

- [ ] **Step 1: Failing tests.** `Test_Between_ForwardGroupsByRelease` (r5→r8, commits spread over r6..r8, CommitCount 4, PullCount 3), `Test_Between_FeatureBranchReleaseInBetween` (r7 changelog has commits the compare lacks → not shown; counts distinct), `Test_Between_RollbackListsWhatLeaves`, `Test_Between_NoLiveIsFirst`, `Test_Between_RegisteredIsUnavailable` (no Compare call), `Test_Between_GitHubDownIsFailedNotError`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `git commit -m "Work out what ships between two releases"`

### Task 5: API — release changelog on `getRelease`

**Files:**
- Modify: `app/release/handlers/releases/releases.go` (Get, DTO), `cmd/controlplane/modules/release.go` (wiring: repository, usecase, subscriber in `group:"subscribers"`, `sweeper.Start(lc, "changelog", u)`)
- Test: `internal/integration/tests/release_changelog_test.go`

**Interfaces:**
- Produces: `ReleaseDTO.Changelog *ChangelogDTO json:"changelog,omitempty"` set only by Get: `{base_release, base_sha, head_sha, state (enum), ahead_by, behind_by, truncated, compare_url, commits: [{sha, title, type, author, committed_at, pull_request: {number,title,url}|null}]}`. `base_release` is the base release number (look up by id, 0 if none).
- Handler port: `Changelogs{Get(ctx, rel entities.Release) (entities.Changelog, error)}`.

- [ ] **Step 1: Failing integration test** `Test_ReleaseChangelog_FrozenAfterBuild`: build r1 and r2 through the fake (existing helpers in `releases_test.go`), fake compare r1..r2 with 2 commits + 1 PR, `GET /api/v1/releases/<r2>` → `changelog.state == "ahead"`, 2 commits, PR number; r1 → `first`.
- [ ] **Step 2: Run** `go test -p 1 ./internal/integration/...` → FAIL. **Step 3: Implement + wire.** **Step 4:** PASS, plus OpenAPI/route-parity tests.
- [ ] **Step 5: Commit** `git commit -m "Return the changelog on a release"`

### Task 6: Deploy — freeze what ships in the change request

**Files:**
- Create: `app/deploy/repositories/changelog/changelog.go` (adapter over `ChangelogsUsecase.Between`)
- Modify: `app/deploy/usecases/deployments/approval.go` (`Proposal.Ships *approval.Ships json:"ships,omitempty"`; `propose` fills it for `deploy`, `promote`, `rollback`; `Preview` sets `preview.Ships = o.Ships`), `deployments.go` (new port `Shipments{Ships(ctx, fromReleaseID, toReleaseID string) (approval.Ships, error)}` injected into `NewDeploymentsUsecase`), `cmd/controlplane/modules/deploy.go`
- Test: `app/deploy/usecases/deployments/ships_test.go`

**Interfaces:**
- Consumes: Task 4 `Between`.
- Produces: `Proposal.Ships`; from = `live.ReleaseID` (empty when nothing live), to = `p.release.ID`.

- [ ] **Step 1: Failing tests** `Test_Propose_FreezesShipsInThePayload` (fake Shipments returns r5→r8; submitted `approval.Change.Proposal.(Proposal).Ships` equals it), `Test_Propose_ConfigApplyHasNoShips`, `Test_Propose_ShipmentErrorStillFilesTheRequest` (fake returns error → request filed, `Ships.State == "failed"`), `Test_Preview_ReturnsFrozenShips`.
- [ ] **Step 2–4:** FAIL → implement → `go test ./app/deploy/...` PASS.
- [ ] **Step 5: Commit** `git commit -m "Freeze what ships in deploy requests"`

### Task 7: API — `preview` on `getRequest`

**Files:**
- Modify: `app/approvals/handlers/requests/dto.go`, `requests.go` (Get calls `Preview`; pending → lines + conflict + ships; otherwise ships only; preview error → `preview` omitted, logged)
- Test: `internal/integration/tests/approvals_test.go`

**Interfaces:**
- Produces: `requestDTO.Preview *previewDTO json:"preview,omitempty"`: `{lines: [{key,before,after}], moved, conflict, ships: {...approval.Ships JSON}}`; `Requests` port gains `Preview(ctx, r) (approval.Preview, error)` (already on the usecase).

- [ ] **Step 1: Failing integration test** `Test_PromoteRequest_ShowsWhatShips`: r1 live on prod, r2 built (fake compare 2 commits), token without bypass promotes → 202 request; `GET /api/v1/requests/:id` → `preview.ships.direction == "forward"`, 2 commits under release 2; approve; executed request still has `preview.ships` and no `preview.lines`.
- [ ] **Step 2–4:** FAIL → implement → PASS (incl. OpenAPI parity, MCP tool schema test).
- [ ] **Step 5: Commit** `git commit -m "Show the preview on change requests over the API"`

### Task 8: Web — release page, list column, request card

**Files:**
- Create: `app/web/templates/pages/release.html`, `app/web/templates/partials/changelog.html`
- Modify: `app/web/handlers/releases/releases.go` (route `GET /services/:service/releases/:number`, `Changes` per row via `Changelogs.Many`), `app/web/templates/pages/releases.html` (Changes column, row links), `app/web/handlers/requests/requests.go` (preview for every status), `app/web/templates/pages/request.html` ("What ships to <env>" / "What leaves <env>" card for every status; lines and conflict only while pending), `app/web/static/app.css` (commit type chips reuse badge styles), `cmd/controlplane/modules/web.go`
- Test: `internal/integration/tests/web_test.go`

**Interfaces:**
- Consumes: `ChangelogsUsecase.Get/Many`, `ReleasesUsecase.Resolve(ctx, slug, "r<n>")`, `DeploymentsUsecase.Live(ctx, slug, env)`.
- Produces: release page per mockup board B: header (status, ref@sha, trigger, age), "Changelog since rN" grouped by PR (commits without PR listed alone), compare link, states `first`/`pending`/`unavailable`/`failed`/`diverged` each with one sentence; side cards Build (commit, image@digest, run link, shared-to services) and "Where it runs" (each env: live release number or "not deployed", releases behind) with a link "Promote from the prod page" to `/services/:service/env/prod` when the release is live on dev and not on prod.

- [ ] **Step 1: Failing web tests** `Test_ReleasePage_ShowsChangelog` (title of a commit and PR `#61` in body), `Test_ReleasesList_LinksToReleasePage` (`/services/api/releases/2` and `2 commits since r1`), `Test_RequestPage_ShowsWhatShipsAfterApproval`.
- [ ] **Step 2–4:** FAIL → implement → PASS. Screenshot via `internal/zzdemo` harness and compare with the mockups.
- [ ] **Step 5: Commit** `git commit -m "Show changelogs on release and request pages"`

### Task 9: Ship

- [ ] `make lint test` green; rewrite history with the layered-commits skill (storage → rules → handlers+wiring+integration), no co-author.
- [ ] Push branch `release-changelog`, open PR, wait for CI, `gh pr merge --rebase`, run the kalzone release workflow, verify on prod: build `kalzone-site` (or `hello`) r+1, `getRelease` shows a changelog, a prod promote request shows `ships`, then cancel it.

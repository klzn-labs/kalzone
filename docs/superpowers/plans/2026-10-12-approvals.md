# Approvals for protected environments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A token with `protected` but without bypass no longer gets `403 err_approval_required` on a protected environment for config, secrets, deploys and queue edits: the call answers `202` with a change request, a human approves or rejects it in the browser, the platform runs it as the token re-applied on the current state, and the agent learns the outcome by polling `/api/v1/requests/:id`.

**Architecture:** `authz.Policy.Gate` tells a usecase whether a change runs (`open`, `bypassed`, `approved`) or `needs_approval`. The config, deploy and queue usecases validate as before and, when approval is needed, hand a pinned proposal to the `submit` usecase of a new `app/approvals` domain and return the resulting `*approval.Pending` as their error; `httperror.Abort` answers it with `202`. The `requests` usecase lets an approver in the browser approve (it rebuilds the token's actor, adds the approval and calls the owning domain through an executor adapter), reject, cancel and sweep expired requests. A Discord webhook is told about each new request in a goroutine. The bypass audit event moves from `Policy.Can` to the usecases, right after the write.

**Tech Stack:** Go 1.26, Gin, Uber fx, GORM on MySQL 8, golang-migrate, testify, htmx templates; integration tests in `internal/integration`.

**Spec:** `docs/superpowers/specs/2026-10-12-approvals-design.md` (all sections). Sub-projects 3–7 are out of scope.

**How the code was checked:** every code block below was compiled and tested in a scratch clone of `b14e2c8`, task by task in this order (`go build ./... && go vet ./...` after each task, `go test -p 1 -count=1 ./...` green at the end, `gofmt -l .` empty, the clean-code hook clean on every Go file including tests). Diffs are against `b14e2c8`; apply them with the shown context, or by hand.

## Global Constraints

- Function bodies ≤ 20 lines, at most 2 levels of nesting, **no comments** in any file: Go, SQL, HTML, CSS, tests (user rule; a hook enforces it). `ALLOW_COMMENTS` is never used.
- One commit per layer, bottom-up: storage (Task 1), rules (Tasks 2–6), handlers + wiring + UI + integration tests (Tasks 7–8), docs (Task 9). No fix/wip commits, no `Co-Authored-By`, do not push. Commit messages follow the repo style (imperative sentence, no prefix).
- Tests run with `-p 1`: the MySQL-backed packages share one test database.
- Kinds: `config.revise`, `secret.set`, `secret.unset`, `deploy`, `promote`, `rollback`, `config.apply`, `queue.create`, `queue.update`, `queue.delete` (`pkg/approval`).
- Subjects (supersede key): `config` for `config.revise`; `secret.NAME` for secrets; `deploy` for deploy, promote, rollback, config apply; `queue.NAME` for queues.
- Statuses: `pending`, `approved`, `executed`, `failed`, `rejected`, `cancelled`, `expired`. Lifetime 24 hours; an `approved` request older than 10 minutes is failed by the sweeper.
- The `202` body: `{"kind":"change_request","id","status":"pending","summary","approve_url":"<base_url>/requests/<id>","poll":"/api/v1/requests/<id>","expires_at"}`.
- Conflicts on approval: config keys `env.NAME`, `secret.NAME`, and `spec` as one unit, moved since the base revision → `409 err_stale`; another deployment live since the request → `err_stale`; queue base version moved → existing `err_conflict`.
- Approvers: maintainer of the request's team or admin, browser session only (`request.approve` has no scope). Cancel: only `requested_by`.
- Queue pause/resume and dead-letter commands keep `403 err_approval_required` without bypass.
- `PLATFORM_APPROVALS_WEBHOOK_URL` (config `approvals.webhook_url`), empty disables; the post is asynchronous with a 10 s timeout, `allowed_mentions.parse = []`, logged errors never contain the URL.
- `token.bypass_used` is published by the usecase after the write via `authz.PublishBypass`; `authz.NewPolicy` takes no publisher.
- Approved domain events carry `request_id` and `approved_by` in their details (`events.WithApproval`).
- Before writing the two templates in Task 8, show the user a mockup (user preference "UI mockups first"); the templates below are the default option.

## Review Focus

1. Two approvers click Approve at the same moment: exactly one execution, the other sees "this request was already decided". Pinned by the conditional transition in Task 1 (`Test_Requests_TransitionOnlyFromTheExpectedStatus`) and Task 6 (`Test_Approve_OnlyByAnApproverWhileStillPending`, approving twice).
2. The token is revoked, or its owner loses maintainer, between request and approval: the run must fail with that error code and change nothing. Task 6 (`Test_Approve_FailsWhenTheTokenNoLongerWorks`) and Task 2 (`Test_Policy_AnApprovalDoesNotReplaceTheProtectedScope`).
3. A secret value must never leave the request path: not in the request row, the summary, the preview, the API, the page or Discord. Task 3 (`Test_SetSecret_NeedingApprovalSealsNowAndNamesOnlyTheKey`, `Test_Preview_HidesSecretValues`) and Task 7 (`Test_ASecretRequestNeverShowsItsValue`).
4. The Discord webhook URL is a secret: a network error must not print it. Task 7 logs `withoutURL(err)`; `Test_Notify_IsQuietWithoutAURLAndSurvivesAFailingWebhook` covers the unreachable and failing cases (read the log line once by hand: it must not contain `/hook`).
5. An approve or reject that arrives after 24 hours but before the next sweep must be refused, not executed. Task 6 (`Test_Approve_OnlyByAnApproverWhileStillPending`, expired branch).

## File Structure

| File | Responsibility |
|---|---|
| `migrations/000006_change_requests.up.sql` / `.down.sql` | `change_requests` |
| `internal/testdb/testdb.go` | table list, close pools after each test |
| `app/approvals/entities/request.go`, `event.go` | request, statuses, transitions, paths, audit event |
| `app/approvals/repositories/mysql/model.go`, `requests.go` | storage |
| `pkg/approval/approval.go` | kinds, `Change`, `Pending`, `Preview`, `Line` |
| `pkg/actor/actor.go`, `pkg/apperror/apperror.go`, `pkg/events/events.go` | `Approval`, `err_stale`, `TargetRequest`, `WithApproval` |
| `app/authz/authz.go`, `token.go` | `Gate`, `Direct`, `PublishBypass`, `request.approve` |
| `app/envconfig/entities/conflict.go`, `entities.go`, `validate.go` | secret change sets, `Keys`, `Conflicts` |
| `app/envconfig/usecases/configs/approval.go`, `revise.go`, `secrets.go`, `configs.go` | gate, proposal, `ApplyApproved`, `Preview` |
| `app/deploy/usecases/deployments/approval.go`, `actions.go`, `schedule.go`, `deployments.go` | same for deploys |
| `app/queue/usecases/queues/approval.go`, `queues.go`, `edit.go`, `delete.go`, `dead.go` | same for queues |
| `app/identity/usecases/tokens/authenticate.go` | `ActorOf` |
| `app/approvals/usecases/submit/submit.go` | create, supersede, notify |
| `app/approvals/usecases/requests/requests.go`, `decide.go`, `sweep.go` | read, approve, reject, cancel, sweep |
| `pkg/config/config.go` | `approvals.webhook_url` |
| `internal/httperror/httperror.go` | `202` for pending, `409` for `err_stale` |
| `app/approvals/repositories/discord/discord.go` | webhook notifier |
| `app/approvals/repositories/{envconfig,deploy,queue}` | executors |
| `app/approvals/handlers/requests/requests.go`, `dto.go` | `/api/v1/requests` |
| `cmd/controlplane/modules/approvals.go`, `authz.go`, `web.go`, `options/options.go` | wiring |
| `internal/integration/suite.go`, `tests/approvals_test.go`, `tests/token_access_test.go` | end-to-end |
| `app/web/handlers/requests/requests.go`, `templates/pages/requests.html`, `request.html`, `partials/components.html`, `layout.html`, `static/app.css`, `activity.go` | UI |
| `README.md` | Approvals section, config row |

---

### Task 1: Storage — change_requests table, request entity and repository

**Files:**
- Create: `migrations/000006_change_requests.up.sql`
- Create: `migrations/000006_change_requests.down.sql`
- Modify: `internal/testdb/testdb.go`
- Create: `app/approvals/entities/request_test.go`
- Create: `app/approvals/entities/request.go`
- Create: `app/approvals/repositories/mysql/requests_test.go`
- Create: `app/approvals/repositories/mysql/model.go`
- Create: `app/approvals/repositories/mysql/requests.go`

The table of spec section 5, the request entity with its pure transitions, and a repository whose `Transition` is a conditional update. `testdb.Open` now closes its pool when the test ends, so the growing integration suite stays under MySQL's 151 connections.

**Interfaces:**
- Produces:

```go
package entities
type Status string
const StatusPending, StatusApproved, StatusExecuted, StatusFailed, StatusRejected, StatusCancelled, StatusExpired Status
const Lifetime = 24 * time.Hour; const ExecutionLimit = 10 * time.Minute; const SummaryLimit = 255; const TextLimit = 500
type Request struct { ID, Kind, Subject, ServiceID, ServiceSlug, TeamID, Environment, Summary string; Payload []byte; RequestedBy, RequesterLogin, TokenID, TokenName string; Status Status; DecidedBy, DeciderLogin string; DecidedAt *time.Time; Reason, Result, ErrorCode, ErrorMessage string; CreatedAt, ExpiresAt time.Time; FinishedAt *time.Time }
type Filter struct { Status Status; TeamIDs []string; Limit int }
func (r Request) Pending() bool
func (r Request) ExpiredAt(now time.Time) bool
func (r Request) Approve(by actor.Actor, at time.Time) Request
func (r Request) Close(status Status, by actor.Actor, reason string, at time.Time) Request
func (r Request) Succeed(result string, at time.Time) Request
func (r Request) Fail(code, message string, at time.Time) Request
func Clip(text string, limit int) string

package mysql
func NewRequestRepository(db *gorm.DB) RequestRepository
func (r RequestRepository) Create(ctx context.Context, req entities.Request) error
func (r RequestRepository) Get(ctx context.Context, id string) (entities.Request, error)
func (r RequestRepository) List(ctx context.Context, f entities.Filter) ([]entities.Request, error)
func (r RequestRepository) PendingFor(ctx context.Context, serviceID, environment, subject string) ([]entities.Request, error)
func (r RequestRepository) Overdue(ctx context.Context, now time.Time) ([]entities.Request, error)
func (r RequestRepository) Transition(ctx context.Context, req entities.Request, from entities.Status) error

package testdb
func Close(gdb *gorm.DB)
```


- [ ] **Step 1: Write the failing tests**

Create `app/approvals/entities/request_test.go`:

```go
package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/stretchr/testify/require"
)

var at = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func Test_Request_ExpiresOnlyWhilePending(t *testing.T) {
	r := entities.Request{Status: entities.StatusPending, ExpiresAt: at}

	require.False(t, r.ExpiredAt(at.Add(-time.Second)))
	require.True(t, r.ExpiredAt(at))
	r.Status = entities.StatusApproved
	require.False(t, r.ExpiredAt(at.Add(time.Hour)))
}

func Test_Request_TransitionsRecordWhoAndWhen(t *testing.T) {
	mia := actor.Actor{UserID: "u-mia", Login: "mia"}
	r := entities.Request{Status: entities.StatusPending}

	approved := r.Approve(mia, at)
	rejected := r.Close(entities.StatusRejected, mia, "not now", at)
	done := approved.Succeed("revision 4", at)
	failed := approved.Fail("err_stale", "env.A changed", at)

	require.Equal(t, entities.StatusApproved, approved.Status)
	require.Equal(t, "mia", approved.DeciderLogin)
	require.Nil(t, approved.FinishedAt)
	require.Equal(t, "not now", rejected.Reason)
	require.Equal(t, &at, rejected.FinishedAt)
	require.Equal(t, "revision 4", done.Result)
	require.Equal(t, "err_stale", failed.ErrorCode)
	require.Equal(t, "mia", failed.DeciderLogin)
}

func Test_Clip_KeepsShortTextAndCutsLongText(t *testing.T) {
	require.Equal(t, "short", entities.Clip("short", 10))
	require.Equal(t, strings.Repeat("a", 9)+"…", entities.Clip(strings.Repeat("a", 20), 10))
}
```

Create `app/approvals/repositories/mysql/requests_test.go`:

```go
package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/approvals/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var (
	ctx     = context.Background()
	created = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	mia     = actor.Actor{UserID: "u-mia", Login: "mia"}
)

func request(id, team, subject string) entities.Request {
	return entities.Request{
		ID: id, Kind: "deploy", Subject: subject, ServiceID: "s1", ServiceSlug: "api", TeamID: team,
		Environment: "prod", Summary: "deploy release #2 to prod", Payload: []byte(`{"release_id": "r2"}`),
		RequestedBy: "u-bob", RequesterLogin: "bob", TokenID: "k1", TokenName: "claude-local",
		Status: entities.StatusPending, CreatedAt: created, ExpiresAt: created.Add(entities.Lifetime),
	}
}

func Test_Requests_RoundTrip(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))

	got, err := repo.Get(ctx, "c1")
	_, missing := repo.Get(ctx, "nope")

	require.NoError(t, err)
	require.JSONEq(t, `{"release_id":"r2"}`, string(got.Payload))
	got.Payload = request("c1", "t1", "deploy").Payload
	require.Equal(t, request("c1", "t1", "deploy"), got)
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_Requests_ListFiltersByStatusAndTeams(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))
	require.NoError(t, repo.Create(ctx, request("c2", "t2", "deploy")))
	require.NoError(t, repo.Transition(ctx, request("c2", "t2", "deploy").Close(entities.StatusRejected, mia, "no", created), entities.StatusPending))

	pending, _ := repo.List(ctx, entities.Filter{Status: entities.StatusPending, Limit: 10})
	t2, _ := repo.List(ctx, entities.Filter{TeamIDs: []string{"t2"}, Limit: 10})
	all, _ := repo.List(ctx, entities.Filter{Limit: 10})

	require.Len(t, pending, 1)
	require.Equal(t, "c1", pending[0].ID)
	require.Len(t, t2, 1)
	require.Equal(t, "no", t2[0].Reason)
	require.Len(t, all, 2)
}

func Test_Requests_TransitionOnlyFromTheExpectedStatus(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	r := request("c1", "t1", "deploy")
	require.NoError(t, repo.Create(ctx, r))

	first := repo.Transition(ctx, r.Approve(mia, created), entities.StatusPending)
	second := repo.Transition(ctx, r.Approve(mia, created), entities.StatusPending)
	got, _ := repo.Get(ctx, "c1")

	require.NoError(t, first)
	require.ErrorIs(t, second, apperror.Conflict(""))
	require.Equal(t, entities.StatusApproved, got.Status)
	require.Equal(t, "mia", got.DeciderLogin)
}

func Test_Requests_PendingForTheSameSubject(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))
	require.NoError(t, repo.Create(ctx, request("c2", "t1", "secret.TOKEN")))

	same, err := repo.PendingFor(ctx, "s1", "prod", "deploy")

	require.NoError(t, err)
	require.Len(t, same, 1)
	require.Equal(t, "c1", same[0].ID)
}

func Test_Requests_OverdueAreExpiredPendingAndStuckApproved(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	now := created.Add(entities.Lifetime)
	stuck := request("c2", "t1", "deploy").Approve(mia, now.Add(-entities.ExecutionLimit))
	fresh := request("c3", "t1", "deploy").Approve(mia, now)
	for _, r := range []entities.Request{request("c1", "t1", "deploy"), stuck, fresh} {
		require.NoError(t, repo.Create(ctx, r))
	}

	overdue, err := repo.Overdue(ctx, now)

	require.NoError(t, err)
	require.Len(t, overdue, 2)
	require.Equal(t, []string{"c1", "c2"}, []string{overdue[0].ID, overdue[1].ID})
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -p 1 -count=1 ./app/approvals/...`
Expected: build failure, `entities.Request` and `mysql.NewRequestRepository` undefined

- [ ] **Step 3: Implement**

Create `migrations/000006_change_requests.up.sql`:

```sql
CREATE TABLE change_requests (
    id CHAR(36) NOT NULL PRIMARY KEY,
    kind VARCHAR(32) NOT NULL,
    subject VARCHAR(160) NOT NULL,
    service_id CHAR(36) NOT NULL,
    service_slug VARCHAR(40) NOT NULL,
    team_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    summary VARCHAR(255) NOT NULL,
    payload JSON NOT NULL,
    requested_by CHAR(36) NOT NULL,
    requester_login VARCHAR(39) NOT NULL,
    token_id CHAR(36) NOT NULL,
    token_name VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    decided_by CHAR(36) NOT NULL DEFAULT '',
    decider_login VARCHAR(39) NOT NULL DEFAULT '',
    decided_at DATETIME(3) NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    result VARCHAR(255) NOT NULL DEFAULT '',
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    finished_at DATETIME(3) NULL,
    KEY change_requests_open (status, expires_at),
    KEY change_requests_subject (service_id, environment, subject, status),
    KEY change_requests_created (created_at)
);
```

Create `migrations/000006_change_requests.down.sql`:

```sql
DROP TABLE change_requests;
```

Modify `internal/testdb/testdb.go` (apply this diff; context lines are the current code):

```diff
--- a/internal/testdb/testdb.go
+++ b/internal/testdb/testdb.go
@@ -17,7 +17,7 @@ import (
 
 const defaultDSN = "root:test@tcp(127.0.0.1:3306)/platform_test"
 
-var tables = []string{"users", "sessions", "teams", "memberships", "services", "config_revisions", "secret_versions", "audit_events", "releases", "deployments", "deployment_events", "agents", "queues", "queue_versions", "queue_publishers", "queue_credentials", "queue_commands", "access_tokens"}
+var tables = []string{"users", "sessions", "teams", "memberships", "services", "config_revisions", "secret_versions", "audit_events", "releases", "deployments", "deployment_events", "agents", "queues", "queue_versions", "queue_publishers", "queue_credentials", "queue_commands", "access_tokens", "change_requests"}
 
 var prepare sync.Once
 
@@ -38,10 +38,17 @@ func Open(t *testing.T) *gorm.DB {
 	prepare.Do(func() { require.NoError(t, createAndMigrate()) })
 	gdb, err := db.NewGormDB(Config())
 	require.NoError(t, err)
+	t.Cleanup(func() { Close(gdb) })
 	Truncate(t, gdb)
 	return gdb
 }
 
+func Close(gdb *gorm.DB) {
+	if sqlDB, err := gdb.DB(); err == nil {
+		_ = sqlDB.Close()
+	}
+}
+
 func Truncate(t *testing.T, gdb *gorm.DB) {
 	t.Helper()
 	for _, table := range tables {
```

Create `app/approvals/entities/request.go`:

```go
package entities

import (
	"time"

	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusExecuted  Status = "executed"
	StatusFailed    Status = "failed"
	StatusRejected  Status = "rejected"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

const (
	Lifetime       = 24 * time.Hour
	ExecutionLimit = 10 * time.Minute
	SummaryLimit   = 255
	TextLimit      = 500
)

type Request struct {
	ID             string
	Kind           string
	Subject        string
	ServiceID      string
	ServiceSlug    string
	TeamID         string
	Environment    string
	Summary        string
	Payload        []byte
	RequestedBy    string
	RequesterLogin string
	TokenID        string
	TokenName      string
	Status         Status
	DecidedBy      string
	DeciderLogin   string
	DecidedAt      *time.Time
	Reason         string
	Result         string
	ErrorCode      string
	ErrorMessage   string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	FinishedAt     *time.Time
}

type Filter struct {
	Status  Status
	TeamIDs []string
	Limit   int
}

func (r Request) Pending() bool {
	return r.Status == StatusPending
}

func (r Request) ExpiredAt(now time.Time) bool {
	return r.Pending() && !now.Before(r.ExpiresAt)
}

func (r Request) Approve(by actor.Actor, at time.Time) Request {
	r.Status, r.DecidedBy, r.DeciderLogin, r.DecidedAt = StatusApproved, by.UserID, by.Login, &at
	return r
}

func (r Request) Close(status Status, by actor.Actor, reason string, at time.Time) Request {
	r.Status, r.DecidedBy, r.DeciderLogin, r.DecidedAt = status, by.UserID, by.Login, &at
	r.Reason, r.FinishedAt = Clip(reason, TextLimit), &at
	return r
}

func (r Request) Succeed(result string, at time.Time) Request {
	r.Status, r.Result, r.FinishedAt = StatusExecuted, Clip(result, SummaryLimit), &at
	return r
}

func (r Request) Fail(code, message string, at time.Time) Request {
	r.Status, r.ErrorCode, r.ErrorMessage, r.FinishedAt = StatusFailed, code, Clip(message, TextLimit), &at
	return r
}

func Clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}
```

Create `app/approvals/repositories/mysql/model.go`:

```go
package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
)

type changeRequest struct {
	ID             string `gorm:"primaryKey"`
	Kind           string
	Subject        string
	ServiceID      string
	ServiceSlug    string
	TeamID         string
	Environment    string
	Summary        string
	Payload        []byte
	RequestedBy    string
	RequesterLogin string
	TokenID        string
	TokenName      string
	Status         string
	DecidedBy      string
	DeciderLogin   string
	DecidedAt      *time.Time
	Reason         string
	Result         string
	ErrorCode      string
	ErrorMessage   string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	FinishedAt     *time.Time
}

func (changeRequest) TableName() string { return "change_requests" }

func fromRequest(r entities.Request) changeRequest {
	return changeRequest{
		ID: r.ID, Kind: r.Kind, Subject: r.Subject, ServiceID: r.ServiceID, ServiceSlug: r.ServiceSlug,
		TeamID: r.TeamID, Environment: r.Environment, Summary: r.Summary, Payload: r.Payload,
		RequestedBy: r.RequestedBy, RequesterLogin: r.RequesterLogin, TokenID: r.TokenID, TokenName: r.TokenName,
		Status: string(r.Status), DecidedBy: r.DecidedBy, DeciderLogin: r.DeciderLogin, DecidedAt: r.DecidedAt,
		Reason: r.Reason, Result: r.Result, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, FinishedAt: r.FinishedAt,
	}
}

func (m changeRequest) toEntity() entities.Request {
	return entities.Request{
		ID: m.ID, Kind: m.Kind, Subject: m.Subject, ServiceID: m.ServiceID, ServiceSlug: m.ServiceSlug,
		TeamID: m.TeamID, Environment: m.Environment, Summary: m.Summary, Payload: m.Payload,
		RequestedBy: m.RequestedBy, RequesterLogin: m.RequesterLogin, TokenID: m.TokenID, TokenName: m.TokenName,
		Status: entities.Status(m.Status), DecidedBy: m.DecidedBy, DeciderLogin: m.DeciderLogin, DecidedAt: utc(m.DecidedAt),
		Reason: m.Reason, Result: m.Result, ErrorCode: m.ErrorCode, ErrorMessage: m.ErrorMessage,
		CreatedAt: m.CreatedAt.UTC(), ExpiresAt: m.ExpiresAt.UTC(), FinishedAt: utc(m.FinishedAt),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
```

Create `app/approvals/repositories/mysql/requests.go`:

```go
package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

var (
	errRequestNotFound = apperror.NotFound("change request not found")
	errRequestChanged  = apperror.Conflict("this request was already decided")
)

type RequestRepository struct {
	db *gorm.DB
}

func NewRequestRepository(db *gorm.DB) RequestRepository {
	return RequestRepository{db: db}
}

func (r RequestRepository) Create(ctx context.Context, req entities.Request) error {
	model := fromRequest(req)
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r RequestRepository) Get(ctx context.Context, id string) (entities.Request, error) {
	var model changeRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Request{}, errRequestNotFound
	}
	return model.toEntity(), err
}

func (r RequestRepository) List(ctx context.Context, f entities.Filter) ([]entities.Request, error) {
	tx := r.db.WithContext(ctx)
	if f.Status != "" {
		tx = tx.Where("status = ?", f.Status)
	}
	if len(f.TeamIDs) > 0 {
		tx = tx.Where("team_id IN ?", f.TeamIDs)
	}
	return find(tx.Order("created_at DESC, id").Limit(f.Limit))
}

func (r RequestRepository) PendingFor(ctx context.Context, serviceID, environment, subject string) ([]entities.Request, error) {
	return find(r.db.WithContext(ctx).Where("service_id = ? AND environment = ? AND subject = ? AND status = ?",
		serviceID, environment, subject, entities.StatusPending).Order("created_at, id"))
}

func (r RequestRepository) Overdue(ctx context.Context, now time.Time) ([]entities.Request, error) {
	return find(r.db.WithContext(ctx).Where("(status = ? AND expires_at <= ?) OR (status = ? AND decided_at <= ?)",
		entities.StatusPending, now, entities.StatusApproved, now.Add(-entities.ExecutionLimit)).Order("created_at, id"))
}

func (r RequestRepository) Transition(ctx context.Context, req entities.Request, from entities.Status) error {
	result := r.db.WithContext(ctx).Model(&changeRequest{}).Where("id = ? AND status = ?", req.ID, from).
		Select("status", "decided_by", "decider_login", "decided_at", "reason", "result", "error_code", "error_message", "finished_at").
		Updates(fromRequest(req))
	if result.Error == nil && result.RowsAffected == 0 {
		return errRequestChanged
	}
	return result.Error
}

func find(tx *gorm.DB) ([]entities.Request, error) {
	var models []changeRequest
	if err := tx.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Request, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/approvals/... ./app/identity/repositories/... && go vet ./...`
Expected: PASS; vet clean

- [ ] **Step 5: Commit**

```bash
git add migrations/000006_change_requests.up.sql migrations/000006_change_requests.down.sql internal/testdb/testdb.go app/approvals
git commit -m "Store change requests for protected environments"
```

---

### Task 2: Rules — gate, approval types, bypass audit out of the policy

**Files:**
- Modify: `app/authz/token_test.go`
- Modify: `app/authz/authz_test.go`
- Modify: `app/envconfig/usecases/configs/secrets_scope_test.go`
- Create: `pkg/approval/approval.go`
- Modify: `pkg/actor/actor.go`
- Modify: `pkg/apperror/apperror.go`
- Modify: `pkg/events/events.go`
- Modify: `app/authz/authz.go`
- Modify: `app/authz/token.go`

`Policy.Gate` returns how a change may run; `Can` maps `needs_approval` to the old `err_approval_required`. The policy stops publishing: `PublishBypass` is called by usecases after their write (Tasks 3–5). `pkg/approval` is the shared kernel between domains and approvals. `NewPolicy` loses its publisher, so the one test outside `app/authz` that builds a policy changes too; the fx provider of `authz.Publisher` is removed in Task 7 (an unused provider is harmless meanwhile).

**Interfaces:**
- Produces:

```go
package approval
const KindConfigRevise, KindSecretSet, KindSecretUnset, KindDeploy, KindPromote, KindRollback, KindConfigApply, KindQueueCreate, KindQueueUpdate, KindQueueDelete string
type Change struct { Kind, Subject, ServiceID, ServiceSlug, TeamID, Environment, Summary string; Proposal any }
type Pending struct { Kind, ID, Status, Summary, ApproveURL, Poll string; ExpiresAt time.Time }
func (p *Pending) Error() string
type Line struct { Key, Before, After string }
type Preview struct { Lines []Line; Moved, Conflict string }

package actor
type Approval struct { RequestID, ApproverID, ApproverLogin string }
func (a Actor) Approved() bool

package apperror
const CodeStale Code = "err_stale"
func Stale(message string) *AppError

package events
const TargetRequest = "request"
func WithApproval(a actor.Actor, details map[string]any) map[string]any

package authz
const ApproveRequest Action = "request.approve"
type Gate string
const Open, Bypassed, Approved, NeedsApproval Gate
func NewPolicy(memberships MembershipReader, environments ProtectionReader) Policy
func (p Policy) Gate(ctx context.Context, a actor.Actor, action Action, r Resource) (Gate, error)
func Direct(g Gate, err error) (Gate, error)
func PublishBypass(ctx context.Context, p Publisher, g Gate, a actor.Actor, action Action, r Resource)
```


- [ ] **Step 1: Write the failing tests**

Modify `app/authz/token_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/authz/token_test.go
+++ b/app/authz/token_test.go
@@ -97,13 +97,58 @@ func asAppError(t *testing.T, err error) *apperror.AppError {
 	return appErr
 }
 
-func Test_Policy_BypassIsAuditedOnlyWhenUsed(t *testing.T) {
+func approvedBy(a actor.Actor, requestID string) actor.Actor {
+	a.Approval = actor.Approval{RequestID: requestID, ApproverID: "maintainer", ApproverLogin: "maintainer"}
+	return a
+}
+
+func Test_Policy_GateSaysHowAProtectedChangeMayRun(t *testing.T) {
+	ctx := context.Background()
+	prod := authz.Resource{TeamID: "places", Environment: "prod"}
+	asking := via("maintainer", only(scope.Deploy, scope.Protected), places, false)
+
+	needs, needsErr := policy.Gate(ctx, asking, authz.Deploy, prod)
+	bypassed, _ := policy.Gate(ctx, via("maintainer", only(scope.Deploy, scope.Protected), places, true), authz.Deploy, prod)
+	approved, _ := policy.Gate(ctx, approvedBy(asking, "c1"), authz.Deploy, prod)
+	dev, _ := policy.Gate(ctx, asking, authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"})
+	session, _ := policy.Gate(ctx, people["maintainer"], authz.Deploy, prod)
+
+	require.NoError(t, needsErr)
+	require.Equal(t, authz.NeedsApproval, needs)
+	require.Equal(t, authz.Bypassed, bypassed)
+	require.Equal(t, authz.Approved, approved)
+	require.Equal(t, authz.Open, dev)
+	require.Equal(t, authz.Open, session)
+}
+
+func Test_Policy_AnApprovalDoesNotReplaceTheProtectedScope(t *testing.T) {
+	a := approvedBy(via("maintainer", only(scope.Deploy), places, false), "c1")
+
+	_, err := policy.Gate(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "prod"})
+
+	requireCode(t, apperror.CodeMissingScope, err)
+}
+
+func Test_Policy_OnlyABrowserSessionApproves(t *testing.T) {
+	ctx := context.Background()
+	team := authz.Resource{TeamID: "places"}
+
+	viaToken := policy.Can(ctx, via("maintainer", scope.All, places, true), authz.ApproveRequest, team)
+	member := policy.Can(ctx, people["member"], authz.ApproveRequest, team)
+
+	requireCode(t, apperror.CodeForbidden, viaToken)
+	requireCode(t, apperror.CodeForbidden, member)
+	require.NoError(t, policy.Can(ctx, people["maintainer"], authz.ApproveRequest, team))
+}
+
+func Test_PublishBypass_OnlyWhenTheGateWasBypassed(t *testing.T) {
 	rec := &recorder{}
-	p := authz.NewPolicy(roles{"places/maintainer": "maintainer"}, protection{"dev": false, "prod": true}, rec)
 	a := via("maintainer", only(scope.Deploy, scope.Protected), places, true)
+	prod := authz.Resource{TeamID: "places", Environment: "prod"}
 
-	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "prod"}))
-	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"}))
+	authz.PublishBypass(context.Background(), rec, authz.Open, a, authz.Deploy, prod)
+	authz.PublishBypass(context.Background(), rec, authz.Approved, a, authz.Deploy, prod)
+	authz.PublishBypass(context.Background(), rec, authz.Bypassed, a, authz.Deploy, prod)
 
 	require.Len(t, rec.events, 1)
 	require.Equal(t, "token.bypass_used", rec.events[0].Action)
```

Modify `app/authz/authz_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/authz/authz_test.go
+++ b/app/authz/authz_test.go
@@ -31,7 +31,6 @@ func (r *recorder) Publish(_ context.Context, e events.Event) { r.events = appen
 var policy = authz.NewPolicy(
 	roles{"places/member": "member", "places/maintainer": "maintainer"},
 	protection{"dev": false, "prod": true},
-	&recorder{},
 )
 
 var people = map[string]actor.Actor{
@@ -65,6 +64,7 @@ var matrix = []rule{
 	{authz.ManageMembers, "", []string{"maintainer", "admin"}},
 	{authz.ManageTeams, "", []string{"admin"}},
 	{authz.ManageAdmins, "", []string{"admin"}},
+	{authz.ApproveRequest, "", []string{"maintainer", "admin"}},
 }
 
 func Test_Policy_Matrix(t *testing.T) {
```

Modify `app/envconfig/usecases/configs/secrets_scope_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/usecases/configs/secrets_scope_test.go
+++ b/app/envconfig/usecases/configs/secrets_scope_test.go
@@ -7,7 +7,6 @@ import (
 	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
-	"github.com/klzn-labs/kalzone/pkg/events"
 	"github.com/klzn-labs/kalzone/pkg/scope"
 	"github.com/stretchr/testify/require"
 )
@@ -25,10 +24,6 @@ type unprotected struct{}
 
 func (unprotected) IsProtected(context.Context, string) (bool, error) { return false, nil }
 
-type discard struct{}
-
-func (discard) Publish(context.Context, events.Event) {}
-
 func withToken(scopes ...scope.Scope) actor.Actor {
 	a := ana
 	a.Token = actor.Token{ID: "tok", Name: "agent", Scopes: scopes, TeamIDs: []string{"t1"}}
@@ -36,7 +31,7 @@ func withToken(scopes ...scope.Scope) actor.Actor {
 }
 
 func Test_Secrets_NeedTheSecretsScopeNotConfig(t *testing.T) {
-	f := newFixtureAuthorizedBy(t, authz.NewPolicy(memberOfOwnerTeam{}, unprotected{}, discard{}))
+	f := newFixtureAuthorizedBy(t, authz.NewPolicy(memberOfOwnerTeam{}, unprotected{}))
 	configOnly, secrets := withToken(scope.Config), withToken(scope.Secrets)
 
 	_, setRefused := f.usecase.SetSecret(ctx, configOnly, "api", "dev", "TOKEN", "x")
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./app/authz/...`
Expected: build failure: `policy.Gate`, `authz.ApproveRequest`, `actor.Approval`, `authz.PublishBypass` undefined; too many arguments to `NewPolicy`

- [ ] **Step 3: Implement**

Create `pkg/approval/approval.go`:

```go
package approval

import "time"

const (
	KindConfigRevise = "config.revise"
	KindSecretSet    = "secret.set"
	KindSecretUnset  = "secret.unset"
	KindDeploy       = "deploy"
	KindPromote      = "promote"
	KindRollback     = "rollback"
	KindConfigApply  = "config.apply"
	KindQueueCreate  = "queue.create"
	KindQueueUpdate  = "queue.update"
	KindQueueDelete  = "queue.delete"
)

type Change struct {
	Kind        string
	Subject     string
	ServiceID   string
	ServiceSlug string
	TeamID      string
	Environment string
	Summary     string
	Proposal    any
}

type Pending struct {
	Kind       string    `json:"kind"`
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	ApproveURL string    `json:"approve_url"`
	Poll       string    `json:"poll"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (p *Pending) Error() string {
	return "change request " + p.ID + " waits for a human to approve it at " + p.ApproveURL
}

type Line struct {
	Key    string
	Before string
	After  string
}

type Preview struct {
	Lines    []Line
	Moved    string
	Conflict string
}
```

Modify `pkg/actor/actor.go` (apply this diff; context lines are the current code):

```diff
--- a/pkg/actor/actor.go
+++ b/pkg/actor/actor.go
@@ -21,11 +21,18 @@ func (t Token) Has(s scope.Scope) bool {
 	return slices.Contains(t.Scopes, s)
 }
 
+type Approval struct {
+	RequestID     string
+	ApproverID    string
+	ApproverLogin string
+}
+
 type Actor struct {
-	UserID string
-	Login  string
-	Admin  bool
-	Token  Token
+	UserID   string
+	Login    string
+	Admin    bool
+	Token    Token
+	Approval Approval
 }
 
 func (a Actor) SignedIn() bool {
@@ -40,6 +47,10 @@ func (a Actor) Sees(teamID string) bool {
 	return !a.ViaToken() || slices.Contains(a.Token.TeamIDs, teamID)
 }
 
+func (a Actor) Approved() bool {
+	return a.Approval.RequestID != ""
+}
+
 func (a Actor) SessionAdmin() bool {
 	return a.Admin && !a.ViaToken()
 }
```

Modify `pkg/apperror/apperror.go` (apply this diff; context lines are the current code):

```diff
--- a/pkg/apperror/apperror.go
+++ b/pkg/apperror/apperror.go
@@ -22,6 +22,7 @@ const (
 	CodeMissingScope     Code = "err_missing_scope"
 	CodeTeamNotGranted   Code = "err_team_not_granted"
 	CodeApprovalRequired Code = "err_approval_required"
+	CodeStale            Code = "err_stale"
 )
 
 type AppError struct {
@@ -59,6 +60,10 @@ func Conflict(message string) *AppError {
 	return New(CodeConflict, message)
 }
 
+func Stale(message string) *AppError {
+	return New(CodeStale, message)
+}
+
 func Forbidden(message string) *AppError {
 	return New(CodeForbidden, message)
 }
```

Modify `pkg/events/events.go` (apply this diff; context lines are the current code):

```diff
--- a/pkg/events/events.go
+++ b/pkg/events/events.go
@@ -3,6 +3,8 @@ package events
 import (
 	"context"
 	"time"
+
+	"github.com/klzn-labs/kalzone/pkg/actor"
 )
 
 type Event struct {
@@ -33,4 +35,16 @@ const (
 	TargetDeployment = "deployment"
 	TargetQueue      = "queue"
 	TargetToken      = "token"
+	TargetRequest    = "request"
 )
+
+func WithApproval(a actor.Actor, details map[string]any) map[string]any {
+	if !a.Approved() {
+		return details
+	}
+	if details == nil {
+		details = map[string]any{}
+	}
+	details["request_id"], details["approved_by"] = a.Approval.RequestID, a.Approval.ApproverLogin
+	return details
+}
```

Modify `app/authz/authz.go` (apply this diff; context lines are the current code):

```diff
--- a/app/authz/authz.go
+++ b/app/authz/authz.go
@@ -24,6 +24,16 @@ const (
 	ManageTeams     Action = "teams.manage"
 	ManageAdmins    Action = "admins.manage"
 	ManageTokens    Action = "tokens.manage"
+	ApproveRequest  Action = "request.approve"
+)
+
+type Gate string
+
+const (
+	Open          Gate = "open"
+	Bypassed      Gate = "bypassed"
+	Approved      Gate = "approved"
+	NeedsApproval Gate = "needs_approval"
 )
 
 type Role string
@@ -60,24 +70,35 @@ type Publisher interface {
 type Policy struct {
 	memberships  MembershipReader
 	environments ProtectionReader
-	publisher    Publisher
 }
 
-func NewPolicy(memberships MembershipReader, environments ProtectionReader, publisher Publisher) Policy {
-	return Policy{memberships: memberships, environments: environments, publisher: publisher}
+func NewPolicy(memberships MembershipReader, environments ProtectionReader) Policy {
+	return Policy{memberships: memberships, environments: environments}
 }
 
 func (p Policy) Can(ctx context.Context, a actor.Actor, action Action, r Resource) error {
+	_, err := Direct(p.Gate(ctx, a, action, r))
+	return err
+}
+
+func (p Policy) Gate(ctx context.Context, a actor.Actor, action Action, r Resource) (Gate, error) {
 	if !a.SignedIn() {
-		return errUnauthenticated
+		return "", errUnauthenticated
 	}
 	if err := p.ownerCan(ctx, a, action, r); err != nil {
-		return err
+		return "", err
 	}
 	if !a.ViaToken() {
-		return nil
+		return Open, nil
+	}
+	return p.tokenGate(ctx, a, action, r)
+}
+
+func Direct(g Gate, err error) (Gate, error) {
+	if err == nil && g == NeedsApproval {
+		return "", errApprovalRequired
 	}
-	return p.tokenCan(ctx, a, action, r)
+	return g, err
 }
 
 func (p Policy) ownerCan(ctx context.Context, a actor.Actor, action Action, r Resource) error {
@@ -95,7 +116,7 @@ func (p Policy) requiredRole(ctx context.Context, action Action, r Resource) (Ro
 	switch action {
 	case CreateService, EditService, CreateRelease:
 		return RoleMember, nil
-	case DeleteService, TransferService, ManageMembers:
+	case DeleteService, TransferService, ManageMembers, ApproveRequest:
 		return RoleMaintainer, nil
 	case EditConfig, EditSecrets, EditQueue, Deploy:
 		return p.roleForEnvironment(ctx, r.Environment)
```

Modify `app/authz/token.go` (apply this diff; context lines are the current code):

```diff
--- a/app/authz/token.go
+++ b/app/authz/token.go
@@ -23,21 +23,21 @@ var (
 	errApprovalRequired = apperror.New(apperror.CodeApprovalRequired, "changes to a protected environment need a human's approval")
 )
 
-func (p Policy) tokenCan(ctx context.Context, a actor.Actor, action Action, r Resource) error {
+func (p Policy) tokenGate(ctx context.Context, a actor.Actor, action Action, r Resource) (Gate, error) {
 	needed, allowed := scopeOf[action]
 	if !allowed {
-		return errNeverForTokens
+		return "", errNeverForTokens
 	}
 	if !a.Token.Has(needed) {
-		return apperror.MissingScope(string(needed))
+		return "", apperror.MissingScope(string(needed))
 	}
 	if err := teamsGranted(a, r.TeamID, r.DestinationTeamID); err != nil {
-		return err
+		return "", err
 	}
 	if !protectable[action] {
-		return nil
+		return Open, nil
 	}
-	return p.protectedRule(ctx, a, action, r)
+	return p.protectedRule(ctx, a, r)
 }
 
 func teamsGranted(a actor.Actor, teamIDs ...string) error {
@@ -49,19 +49,31 @@ func teamsGranted(a actor.Actor, teamIDs ...string) error {
 	return nil
 }
 
-func (p Policy) protectedRule(ctx context.Context, a actor.Actor, action Action, r Resource) error {
+func (p Policy) protectedRule(ctx context.Context, a actor.Actor, r Resource) (Gate, error) {
 	protected, err := p.environments.IsProtected(ctx, r.Environment)
 	if err != nil || !protected {
-		return err
+		return Open, err
 	}
 	if !a.Token.Has(scope.Protected) {
-		return apperror.MissingScope(string(scope.Protected))
+		return "", apperror.MissingScope(string(scope.Protected))
 	}
-	if !a.Token.Bypass {
-		return errApprovalRequired
+	return protectedGate(a), nil
+}
+
+func protectedGate(a actor.Actor) Gate {
+	switch {
+	case a.Approved():
+		return Approved
+	case a.Token.Bypass:
+		return Bypassed
+	}
+	return NeedsApproval
+}
+
+func PublishBypass(ctx context.Context, p Publisher, g Gate, a actor.Actor, action Action, r Resource) {
+	if g == Bypassed {
+		p.Publish(ctx, bypassEvent(a, action, r))
 	}
-	p.publisher.Publish(ctx, bypassEvent(a, action, r))
-	return nil
 }
 
 func bypassEvent(a actor.Actor, action Action, r Resource) events.Event {
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/authz/... ./pkg/... ./app/envconfig/... && go vet ./...`
Expected: PASS

---

### Task 3: Rules — config and secrets become requests

**Files:**
- Create: `app/envconfig/entities/conflict_test.go`
- Modify: `app/envconfig/entities/entities.go`
- Modify: `app/envconfig/entities/validate.go`
- Create: `app/envconfig/entities/conflict.go`
- Modify: `app/envconfig/usecases/configs/fakes_test.go`
- Create: `app/envconfig/usecases/configs/approval_test.go`
- Modify: `app/envconfig/usecases/configs/configs.go`
- Modify: `app/envconfig/usecases/configs/revise.go`
- Modify: `app/envconfig/usecases/configs/secrets.go`
- Create: `app/envconfig/usecases/configs/approval.go`

A `ChangeSet` can now set and unset secret versions, so revise, set secret and unset secret share one path: `editable` (gate) → `change` (apply, validate, no-op check) → `propose` or `commit`. `Revise` drops secret fields an API caller might send. `ApplyApproved` re-applies a proposal on the current revision after checking `Conflicts` against its base. `Preview` diffs the proposal against now and never shows secret values.

**Interfaces:**
- Consumes: `authz.Gate`, `authz.NeedsApproval`, `authz.PublishBypass`, `approval.Change`, `approval.Pending`, `approval.Preview`, `approval.Line`, `apperror.Stale`, `events.WithApproval` (Task 2).
- Produces:

```go
package entities
type ChangeSet struct { Spec *Spec; SetEnv map[string]string; UnsetEnv []string; SetSecrets map[string]string; UnsetSecrets []string; Message string }
func (c ChangeSet) Keys() []string
func Conflicts(c ChangeSet, base, current Revision) []string

package configs
type Authorizer interface { Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error) }
type Requests interface { Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error) }
func NewConfigUsecase(cfg config.Config, store Store, services ServiceLookup, environments EnvironmentLookup, sealer Sealer, authz Authorizer, publisher Publisher, requests Requests) ConfigUsecase
type Proposal struct { Kind, Service, Environment string; Base int; Change entities.ChangeSet }
func (u ConfigUsecase) ApplyApproved(ctx context.Context, a actor.Actor, p Proposal) (entities.Revision, error)
func (u ConfigUsecase) Preview(ctx context.Context, p Proposal) (approval.Preview, error)
```


- [ ] **Step 1: Write the failing tests**

Create `app/envconfig/entities/conflict_test.go`:

```go
package entities_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/stretchr/testify/require"
)

func revisionWith(env map[string]string, secrets map[string]string) entities.Revision {
	r := entities.Initial("svc", "prod")
	for k, v := range env {
		r.Env[k] = v
	}
	for k, v := range secrets {
		r.Secrets[k] = v
	}
	return r
}

func Test_ChangeSet_KeysNameEveryTouchedKeyOnce(t *testing.T) {
	spec := entities.DefaultSpec()
	cs := entities.ChangeSet{
		Spec: &spec, SetEnv: map[string]string{"B": "1", "A": "2"}, UnsetEnv: []string{"A"},
		SetSecrets: map[string]string{"TOKEN": "v2"}, UnsetSecrets: []string{"OLD"},
	}

	require.Equal(t, []string{"env.A", "env.B", "secret.OLD", "secret.TOKEN", "spec"}, cs.Keys())
}

func Test_ApplyTo_SetsAndUnsetsSecretVersions(t *testing.T) {
	base := revisionWith(nil, map[string]string{"OLD": "v0"})
	cs := entities.ChangeSet{SetSecrets: map[string]string{"TOKEN": "v1"}, UnsetSecrets: []string{"OLD"}}

	next := cs.ApplyTo(base.Next())

	require.Equal(t, map[string]string{"TOKEN": "v1"}, next.Secrets)
}

func Test_Conflicts_OnlyKeysTheRequestTouchesAndThatMovedSinceItsBase(t *testing.T) {
	base := revisionWith(map[string]string{"A": "1", "B": "1"}, nil)
	current := revisionWith(map[string]string{"A": "9", "B": "1", "C": "1"}, nil)
	current.Spec.Replicas = 3

	clean := entities.Conflicts(entities.ChangeSet{SetEnv: map[string]string{"B": "2"}}, base, current)
	clash := entities.Conflicts(entities.ChangeSet{SetEnv: map[string]string{"A": "2"}, UnsetEnv: []string{"C"}}, base, current)
	spec := entities.DefaultSpec()
	wholeSpec := entities.Conflicts(entities.ChangeSet{Spec: &spec}, base, current)

	require.Empty(t, clean)
	require.Equal(t, []string{"env.A", "env.C"}, clash)
	require.Equal(t, []string{"spec"}, wholeSpec)
}
```

Modify `app/envconfig/usecases/configs/fakes_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/usecases/configs/fakes_test.go
+++ b/app/envconfig/usecases/configs/fakes_test.go
@@ -10,6 +10,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/config"
 	"github.com/klzn-labs/kalzone/pkg/events"
 )
@@ -110,14 +111,25 @@ func (s *sealer) Open(ciphertext, _, additionalData []byte, _ string) ([]byte, e
 type checks struct {
 	resources []authz.Resource
 	denyEnv   string
+	gate      authz.Gate
 }
 
-func (c *checks) Can(_ context.Context, _ actor.Actor, _ authz.Action, r authz.Resource) error {
+func (c *checks) Gate(_ context.Context, _ actor.Actor, _ authz.Action, r authz.Resource) (authz.Gate, error) {
 	c.resources = append(c.resources, r)
 	if r.Environment == c.denyEnv {
-		return apperror.Forbidden("no")
+		return "", apperror.Forbidden("no")
 	}
-	return nil
+	if c.gate == "" {
+		return authz.Open, nil
+	}
+	return c.gate, nil
+}
+
+type requests struct{ changes []approval.Change }
+
+func (r *requests) Submit(_ context.Context, _ actor.Actor, c approval.Change) (*approval.Pending, error) {
+	r.changes = append(r.changes, c)
+	return &approval.Pending{ID: "c1", Status: "pending"}, nil
 }
 
 type published struct{ events []events.Event }
@@ -125,11 +137,12 @@ type published struct{ events []events.Event }
 func (p *published) Publish(_ context.Context, e events.Event) { p.events = append(p.events, e) }
 
 type fixture struct {
-	store   *store
-	sealer  *sealer
-	checks  *checks
-	events  *published
-	usecase configs.ConfigUsecase
+	store    *store
+	sealer   *sealer
+	checks   *checks
+	events   *published
+	requests *requests
+	usecase  configs.ConfigUsecase
 }
 
 func newFixture(t *testing.T) fixture {
@@ -139,10 +152,10 @@ func newFixture(t *testing.T) fixture {
 
 func newFixtureAuthorizedBy(t *testing.T, authorizer configs.Authorizer) fixture {
 	t.Helper()
-	f := fixture{store: &store{}, sealer: &sealer{}, events: &published{}}
+	f := fixture{store: &store{}, sealer: &sealer{}, events: &published{}, requests: &requests{}}
 	f.checks, _ = authorizer.(*checks)
 	cfg := config.Config{AllowedDomains: []string{"kalz.one"}}
-	f.usecase = configs.NewConfigUsecase(cfg, f.store, services{}, environments{}, f.sealer, authorizer, f.events)
+	f.usecase = configs.NewConfigUsecase(cfg, f.store, services{}, environments{}, f.sealer, authorizer, f.events, f.requests)
 	return f
 }
 
```

Create `app/envconfig/usecases/configs/approval_test.go`:

```go
package configs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

var approvedAna = actor.Actor{UserID: "ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverID: "mia", ApproverLogin: "mia"}}

func setEnv(key, value string) entities.ChangeSet {
	return entities.ChangeSet{SetEnv: map[string]string{key: value}}
}

func proposalOf(t *testing.T, c approval.Change) configs.Proposal {
	t.Helper()
	raw, err := json.Marshal(c.Proposal)
	require.NoError(t, err)
	var p configs.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

func Test_Revise_NeedingApprovalSubmitsTheChangeInsteadOfCommitting(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, err := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Empty(t, f.store.revisions)
	c := f.requests.changes[0]
	require.Equal(t, approval.KindConfigRevise, c.Kind)
	require.Equal(t, "config", c.Subject)
	require.Equal(t, "change env.A", c.Summary)
	require.Equal(t, configs.Proposal{Kind: approval.KindConfigRevise, Service: "api", Environment: "prod", Change: setEnv("A", "1")}, proposalOf(t, c))
}

func Test_Revise_NeedingApprovalValidatesFirst(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, invalid := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("lower", "1"))
	_, noop := f.usecase.Revise(ctx, ana, "api", "prod", entities.ChangeSet{})

	require.ErrorIs(t, invalid, apperror.Invalid(""))
	require.ErrorIs(t, noop, apperror.Invalid(""))
	require.Empty(t, f.requests.changes)
}

func Test_SetSecret_NeedingApprovalSealsNowAndNamesOnlyTheKey(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval

	_, err := f.usecase.SetSecret(ctx, ana, "api", "prod", "TOKEN", "hunter2")

	require.Error(t, err)
	require.Len(t, f.store.versions, 1)
	c := f.requests.changes[0]
	require.Equal(t, "secret.TOKEN", c.Subject)
	require.Equal(t, "set secret.TOKEN", c.Summary)
	require.Equal(t, map[string]string{"TOKEN": f.store.versions[0].ID}, proposalOf(t, c).Change.SetSecrets)
	require.NotContains(t, fmt.Sprint(c), "hunter2")
}

func requestThenMove(t *testing.T, f fixture, request, meanwhile entities.ChangeSet) configs.Proposal {
	t.Helper()
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.Revise(ctx, ana, "api", "prod", request)
	f.checks.gate = authz.Open
	_, err := f.usecase.Revise(ctx, ana, "api", "prod", meanwhile)
	require.NoError(t, err)
	f.checks.gate = authz.Approved
	return proposalOf(t, f.requests.changes[0])
}

func Test_ApplyApproved_ReappliesTheChangeOnTheCurrentRevision(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("B", "2"), setEnv("A", "9"))

	rev, err := f.usecase.ApplyApproved(ctx, approvedAna, p)

	require.NoError(t, err)
	require.Equal(t, 2, rev.Number)
	require.Equal(t, map[string]string{"A": "9", "B": "2"}, rev.Env)
	last := f.events.events[len(f.events.events)-1]
	require.Equal(t, "c1", last.Details["request_id"])
	require.Equal(t, "mia", last.Details["approved_by"])
}

func Test_ApplyApproved_FailsWhenTheSameKeyMoved(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("A", "2"), setEnv("A", "9"))

	_, err := f.usecase.ApplyApproved(ctx, approvedAna, p)

	require.Equal(t, apperror.CodeStale, apperror.CodeOf(err))
	require.Contains(t, err.Error(), "env.A")
	require.Len(t, f.store.revisions, 1)
}

func Test_Preview_DiffsAgainstTheCurrentRevisionAndWarns(t *testing.T) {
	f := newFixture(t)
	p := requestThenMove(t, f, setEnv("A", "2"), setEnv("A", "9"))

	preview, err := f.usecase.Preview(ctx, p)

	require.NoError(t, err)
	require.Equal(t, []approval.Line{{Key: "env.A", Before: "9", After: "2"}}, preview.Lines)
	require.Contains(t, preview.Moved, "from revision 0 to 1")
	require.Contains(t, preview.Conflict, "env.A")
}

func Test_Preview_HidesSecretValues(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.NeedsApproval
	_, _ = f.usecase.SetSecret(ctx, ana, "api", "prod", "TOKEN", "hunter2")

	preview, err := f.usecase.Preview(ctx, proposalOf(t, f.requests.changes[0]))

	require.NoError(t, err)
	require.Equal(t, []approval.Line{{Key: "secret.TOKEN", After: "new value"}}, preview.Lines)
	require.Empty(t, preview.Moved)
}

func Test_Revise_PublishesTheBypassOnlyWhenTheChangeIsMade(t *testing.T) {
	f := newFixture(t)
	f.checks.gate = authz.Bypassed

	_, invalid := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("lower", "1"))
	_, err := f.usecase.Revise(ctx, ana, "api", "prod", setEnv("A", "1"))

	require.Error(t, invalid)
	require.NoError(t, err)
	require.Len(t, f.events.events, 2)
	require.Equal(t, "config.revised", f.events.events[0].Action)
	require.Equal(t, "token.bypass_used", f.events.events[1].Action)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./app/envconfig/...`
Expected: build failure: `configs.Proposal`, `ChangeSet.SetSecrets`, `entities.Conflicts` undefined; too many arguments to `NewConfigUsecase`

- [ ] **Step 3: Implement**

Modify `app/envconfig/entities/entities.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/entities/entities.go
+++ b/app/envconfig/entities/entities.go
@@ -75,10 +75,12 @@ type SecretInfo struct {
 }
 
 type ChangeSet struct {
-	Spec     *Spec
-	SetEnv   map[string]string
-	UnsetEnv []string
-	Message  string
+	Spec         *Spec             `json:"spec,omitempty"`
+	SetEnv       map[string]string `json:"set_env,omitempty"`
+	UnsetEnv     []string          `json:"unset_env,omitempty"`
+	SetSecrets   map[string]string `json:"set_secrets,omitempty"`
+	UnsetSecrets []string          `json:"unset_secrets,omitempty"`
+	Message      string            `json:"message,omitempty"`
 }
 
 type ServiceRef struct {
```

Modify `app/envconfig/entities/validate.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/entities/validate.go
+++ b/app/envconfig/entities/validate.go
@@ -120,6 +120,12 @@ func (c ChangeSet) ApplyTo(r Revision) Revision {
 	for _, name := range c.UnsetEnv {
 		delete(r.Env, name)
 	}
+	for name, versionID := range c.SetSecrets {
+		r.Secrets[name] = versionID
+	}
+	for _, name := range c.UnsetSecrets {
+		delete(r.Secrets, name)
+	}
 	r.Message = strings.TrimSpace(c.Message)
 	return r
 }
```

Create `app/envconfig/entities/conflict.go`:

```go
package entities

import (
	"slices"
	"strings"
)

func (c ChangeSet) Keys() []string {
	keys := []string{}
	if c.Spec != nil {
		keys = append(keys, "spec")
	}
	keys = prefixed(keys, "env.", unionKeys(c.SetEnv, nil), c.UnsetEnv)
	keys = prefixed(keys, "secret.", unionKeys(c.SetSecrets, nil), c.UnsetSecrets)
	slices.Sort(keys)
	return slices.Compact(keys)
}

func prefixed(keys []string, prefix string, groups ...[]string) []string {
	for _, group := range groups {
		for _, name := range group {
			keys = append(keys, prefix+name)
		}
	}
	return keys
}

func Conflicts(c ChangeSet, base, current Revision) []string {
	moved := map[string]bool{}
	for _, key := range ChangedKeys(Diff(base, current)) {
		moved[unitOf(key)] = true
	}
	return slices.DeleteFunc(c.Keys(), func(key string) bool { return !moved[key] })
}

func unitOf(key string) string {
	if strings.HasPrefix(key, "spec.") {
		return "spec"
	}
	return key
}
```

Modify `app/envconfig/usecases/configs/configs.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/usecases/configs/configs.go
+++ b/app/envconfig/usecases/configs/configs.go
@@ -6,6 +6,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/app/envconfig/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/config"
 	"github.com/klzn-labs/kalzone/pkg/events"
 )
@@ -35,7 +36,11 @@ type Sealer interface {
 }
 
 type Authorizer interface {
-	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
+	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
+}
+
+type Requests interface {
+	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
 }
 
 type Publisher interface {
@@ -49,13 +54,14 @@ type ConfigUsecase struct {
 	sealer         Sealer
 	authz          Authorizer
 	publisher      Publisher
+	requests       Requests
 	allowedDomains []string
 }
 
-func NewConfigUsecase(cfg config.Config, store Store, services ServiceLookup, environments EnvironmentLookup, sealer Sealer, authz Authorizer, publisher Publisher) ConfigUsecase {
+func NewConfigUsecase(cfg config.Config, store Store, services ServiceLookup, environments EnvironmentLookup, sealer Sealer, authz Authorizer, publisher Publisher, requests Requests) ConfigUsecase {
 	return ConfigUsecase{
 		store: store, services: services, environments: environments, sealer: sealer,
-		authz: authz, publisher: publisher, allowedDomains: cfg.AllowedDomains,
+		authz: authz, publisher: publisher, requests: requests, allowedDomains: cfg.AllowedDomains,
 	}
 }
 
@@ -64,6 +70,10 @@ type target struct {
 	environment string
 }
 
+func (t target) resource() authz.Resource {
+	return authz.Resource{TeamID: t.service.OwnerTeamID, Environment: t.environment}
+}
+
 func (u ConfigUsecase) resolve(ctx context.Context, serviceSlug, environment string) (target, error) {
 	service, err := u.services.FindService(ctx, serviceSlug)
 	if err != nil {
```

Modify `app/envconfig/usecases/configs/revise.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/usecases/configs/revise.go
+++ b/app/envconfig/usecases/configs/revise.go
@@ -7,6 +7,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/envconfig/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/clock"
 	"github.com/klzn-labs/kalzone/pkg/events"
 	"github.com/klzn-labs/kalzone/pkg/uuid"
@@ -19,29 +20,48 @@ var (
 	errNoChanges      = apperror.Invalid("nothing changed")
 )
 
+type edit struct {
+	target
+	current entities.Revision
+	action  authz.Action
+	gate    authz.Gate
+}
+
 func (u ConfigUsecase) Revise(ctx context.Context, a actor.Actor, serviceSlug, environment string, cs entities.ChangeSet) (entities.Revision, error) {
-	t, current, err := u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)
+	e, err := u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)
 	if err != nil {
 		return entities.Revision{}, err
 	}
-	next := cs.ApplyTo(current.Next())
-	if err := u.validate(ctx, t, next); err != nil {
-		return entities.Revision{}, err
-	}
-	return u.commit(ctx, a, t, current, next)
+	cs.SetSecrets, cs.UnsetSecrets = nil, nil
+	return u.change(ctx, a, e, approval.KindConfigRevise, cs)
 }
 
-func (u ConfigUsecase) editable(ctx context.Context, a actor.Actor, action authz.Action, serviceSlug, environment string) (target, entities.Revision, error) {
+func (u ConfigUsecase) editable(ctx context.Context, a actor.Actor, action authz.Action, serviceSlug, environment string) (edit, error) {
 	t, err := u.resolve(ctx, serviceSlug, environment)
 	if err != nil {
-		return target{}, entities.Revision{}, err
+		return edit{}, err
 	}
-	resource := authz.Resource{TeamID: t.service.OwnerTeamID, Environment: environment}
-	if err := u.authz.Can(ctx, a, action, resource); err != nil {
-		return target{}, entities.Revision{}, err
+	gate, err := u.authz.Gate(ctx, a, action, t.resource())
+	if err != nil {
+		return edit{}, err
 	}
 	current, err := u.current(ctx, t)
-	return t, current, err
+	return edit{target: t, current: current, action: action, gate: gate}, err
+}
+
+func (u ConfigUsecase) change(ctx context.Context, a actor.Actor, e edit, kind string, cs entities.ChangeSet) (entities.Revision, error) {
+	next := cs.ApplyTo(e.current.Next())
+	if err := u.validate(ctx, e.target, next); err != nil {
+		return entities.Revision{}, err
+	}
+	changes := entities.Diff(e.current, next)
+	if len(changes) == 0 {
+		return entities.Revision{}, errNoChanges
+	}
+	if e.gate == authz.NeedsApproval {
+		return entities.Revision{}, u.propose(ctx, a, e, kind, cs, changes)
+	}
+	return u.commit(ctx, a, e, next, changes)
 }
 
 func (u ConfigUsecase) validate(ctx context.Context, t target, next entities.Revision) error {
@@ -78,16 +98,13 @@ func (u ConfigUsecase) internalFree(ctx context.Context, t target) error {
 	return errInternalTaken
 }
 
-func (u ConfigUsecase) commit(ctx context.Context, a actor.Actor, t target, current, next entities.Revision) (entities.Revision, error) {
-	changes := entities.Diff(current, next)
-	if len(changes) == 0 {
-		return entities.Revision{}, errNoChanges
-	}
+func (u ConfigUsecase) commit(ctx context.Context, a actor.Actor, e edit, next entities.Revision, changes []entities.Change) (entities.Revision, error) {
 	next.ID, next.AuthorID, next.CreatedAt = uuid.NewUUID(), a.UserID, clock.Now().UTC()
 	if err := u.store.Insert(ctx, next); err != nil {
 		return entities.Revision{}, err
 	}
-	u.publisher.Publish(ctx, revisedEvent(a, t, next, changes))
+	u.publisher.Publish(ctx, revisedEvent(a, e.target, next, changes))
+	authz.PublishBypass(ctx, u.publisher, e.gate, a, e.action, e.resource())
 	return next, nil
 }
 
@@ -96,6 +113,6 @@ func revisedEvent(a actor.Actor, t target, rev entities.Revision, changes []enti
 		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: "config.revised",
 		TargetKind: events.TargetConfig, TargetID: t.service.ID, TargetName: t.service.Slug,
 		TeamID: t.service.OwnerTeamID, Environment: t.environment,
-		Details: map[string]any{"revision": rev.Number, "changed": entities.ChangedKeys(changes), "message": rev.Message},
+		Details: events.WithApproval(a, map[string]any{"revision": rev.Number, "changed": entities.ChangedKeys(changes), "message": rev.Message}),
 	}
 }
```

Modify `app/envconfig/usecases/configs/secrets.go` (apply this diff; context lines are the current code):

```diff
--- a/app/envconfig/usecases/configs/secrets.go
+++ b/app/envconfig/usecases/configs/secrets.go
@@ -8,6 +8,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/envconfig/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/clock"
 	"github.com/klzn-labs/kalzone/pkg/uuid"
 )
@@ -18,35 +19,28 @@ func (u ConfigUsecase) SetSecret(ctx context.Context, a actor.Actor, serviceSlug
 	if err := validateSecret(name, value); err != nil {
 		return entities.Revision{}, err
 	}
-	t, current, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
+	e, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
 	if err != nil {
 		return entities.Revision{}, err
 	}
-	version, err := u.seal(ctx, a, t, name, value)
+	version, err := u.seal(ctx, a, e.target, name, value)
 	if err != nil {
 		return entities.Revision{}, err
 	}
-	next := current.Next()
-	next.Secrets[name] = version.ID
-	next.Message = "set secret " + name
-	if err := next.Validate(); err != nil {
-		return entities.Revision{}, err
-	}
-	return u.commit(ctx, a, t, current, next)
+	cs := entities.ChangeSet{SetSecrets: map[string]string{name: version.ID}, Message: "set secret " + name}
+	return u.change(ctx, a, e, approval.KindSecretSet, cs)
 }
 
 func (u ConfigUsecase) UnsetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name string) (entities.Revision, error) {
-	t, current, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
+	e, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
 	if err != nil {
 		return entities.Revision{}, err
 	}
-	if _, exists := current.Secrets[name]; !exists {
+	if _, exists := e.current.Secrets[name]; !exists {
 		return entities.Revision{}, errSecretNotFound
 	}
-	next := current.Next()
-	delete(next.Secrets, name)
-	next.Message = "removed secret " + name
-	return u.commit(ctx, a, t, current, next)
+	cs := entities.ChangeSet{UnsetSecrets: []string{name}, Message: "removed secret " + name}
+	return u.change(ctx, a, e, approval.KindSecretUnset, cs)
 }
 
 func validateSecret(name, value string) error {
```

Create `app/envconfig/usecases/configs/approval.go`:

```go
package configs

import (
	"context"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind        string             `json:"kind"`
	Service     string             `json:"service"`
	Environment string             `json:"environment"`
	Base        int                `json:"base"`
	Change      entities.ChangeSet `json:"change"`
}

var verbs = map[string]string{approval.KindConfigRevise: "change", approval.KindSecretSet: "set", approval.KindSecretUnset: "remove"}

func (u ConfigUsecase) propose(ctx context.Context, a actor.Actor, e edit, kind string, cs entities.ChangeSet, changes []entities.Change) error {
	keys := entities.ChangedKeys(changes)
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: kind, Subject: subjectOf(kind, keys), ServiceID: e.service.ID, ServiceSlug: e.service.Slug,
		TeamID: e.service.OwnerTeamID, Environment: e.environment, Summary: verbs[kind] + " " + strings.Join(keys, ", "),
		Proposal: Proposal{Kind: kind, Service: e.service.Slug, Environment: e.environment, Base: e.current.Number, Change: cs},
	})
	if err != nil {
		return err
	}
	return pending
}

func subjectOf(kind string, keys []string) string {
	if kind == approval.KindConfigRevise {
		return "config"
	}
	return keys[0]
}

func (u ConfigUsecase) ApplyApproved(ctx context.Context, a actor.Actor, p Proposal) (entities.Revision, error) {
	e, err := u.editable(ctx, a, actionOf(p.Kind), p.Service, p.Environment)
	if err != nil {
		return entities.Revision{}, err
	}
	if err := u.unmoved(ctx, e, p); err != nil {
		return entities.Revision{}, err
	}
	return u.change(ctx, a, e, p.Kind, p.Change)
}

func actionOf(kind string) authz.Action {
	if kind == approval.KindConfigRevise {
		return authz.EditConfig
	}
	return authz.EditSecrets
}

func (u ConfigUsecase) unmoved(ctx context.Context, e edit, p Proposal) error {
	conflicts, err := u.conflicts(ctx, e.target, e.current, p)
	if err != nil || len(conflicts) == 0 {
		return err
	}
	return apperror.Stale("changed since the request: " + strings.Join(conflicts, ", "))
}

func (u ConfigUsecase) conflicts(ctx context.Context, t target, current entities.Revision, p Proposal) ([]string, error) {
	base, err := u.revision(ctx, t.service.ID, t.environment, p.Base)
	if err != nil {
		return nil, err
	}
	return entities.Conflicts(p.Change, base, current), nil
}

func (u ConfigUsecase) Preview(ctx context.Context, p Proposal) (approval.Preview, error) {
	t, err := u.resolve(ctx, p.Service, p.Environment)
	if err != nil {
		return approval.Preview{}, err
	}
	current, err := u.current(ctx, t)
	if err != nil {
		return approval.Preview{}, err
	}
	conflicts, err := u.conflicts(ctx, t, current, p)
	return previewOf(current, p, conflicts), err
}

func previewOf(current entities.Revision, p Proposal, conflicts []string) approval.Preview {
	preview := approval.Preview{Lines: linesOf(entities.Diff(current, p.Change.ApplyTo(current.Next())))}
	if current.Number != p.Base {
		preview.Moved = fmt.Sprintf("the config moved from revision %d to %d since the request", p.Base, current.Number)
	}
	if len(conflicts) > 0 {
		preview.Conflict = "approving will fail: " + strings.Join(conflicts, ", ") + " changed since the request"
	}
	return preview
}

func linesOf(changes []entities.Change) []approval.Line {
	lines := make([]approval.Line, 0, len(changes))
	for _, c := range changes {
		lines = append(lines, lineOf(c))
	}
	return lines
}

func lineOf(c entities.Change) approval.Line {
	line := approval.Line{Key: c.Section + "." + c.Key, Before: c.Before, After: c.After}
	if c.Section == "secret" {
		line.Before, line.After = secretStates(c.Kind)
	}
	return line
}

func secretStates(kind entities.ChangeKind) (string, string) {
	switch kind {
	case entities.Added:
		return "", "new value"
	case entities.Removed:
		return "set", "removed"
	}
	return "set", "new value"
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/envconfig/... && go vet ./...`
Expected: PASS, including the existing revise and secret tests

---

### Task 4: Rules — deploys, promotions, rollbacks and config applications become requests

**Files:**
- Modify: `app/deploy/usecases/deployments/deployments_test.go`
- Create: `app/deploy/usecases/deployments/approval_test.go`
- Modify: `app/deploy/usecases/deployments/deployments.go`
- Modify: `app/deploy/usecases/deployments/schedule.go`
- Modify: `app/deploy/usecases/deployments/actions.go`
- Create: `app/deploy/usecases/deployments/approval.go`

`authorize` now returns a `plan` that carries the actor and the gate; each action fills the plan and calls `proceed`, which schedules or proposes. The proposal stores the live deployment id; promote stores the release it resolved. `ApplyApproved` fails with `err_stale` when another deployment went live, then replays the action; a pinned promote goes through `promotePinned`. `schedule` publishes the bypass event with the deployment.

**Interfaces:**
- Consumes: Task 2 types.
- Produces:

```go
package deployments
type Authorizer interface { Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error) }
type Requests interface { Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error) }
func NewDeploymentsUsecase(store Store, services ServiceLookup, releases ReleaseLookup, configs ConfigSource, environments EnvironmentLookup, authz Authorizer, publisher Publisher, requests Requests) DeploymentsUsecase
type Proposal struct { Kind, Service, Environment, ReleaseID, Strategy, From, DeploymentID, LiveID string }
func (u DeploymentsUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error)
func (u DeploymentsUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error)
```


- [ ] **Step 1: Write the failing tests**

Modify `app/deploy/usecases/deployments/deployments_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/deploy/usecases/deployments/deployments_test.go
+++ b/app/deploy/usecases/deployments/deployments_test.go
@@ -11,6 +11,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/events"
 	"github.com/stretchr/testify/require"
 )
@@ -41,13 +42,26 @@ func (environments) EnvironmentExists(_ context.Context, slug string) error {
 	return nil
 }
 
-type policy struct{ denyEnv string }
+type policy struct {
+	denyEnv string
+	gate    authz.Gate
+}
 
-func (p policy) Can(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) error {
+func (p policy) Gate(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error) {
 	if action != authz.Deploy || r.Environment == p.denyEnv {
-		return apperror.Forbidden("no")
+		return "", apperror.Forbidden("no")
 	}
-	return nil
+	if p.gate == "" {
+		return authz.Open, nil
+	}
+	return p.gate, nil
+}
+
+type requests struct{ changes []approval.Change }
+
+func (r *requests) Submit(_ context.Context, _ actor.Actor, c approval.Change) (*approval.Pending, error) {
+	r.changes = append(r.changes, c)
+	return &approval.Pending{ID: "c1", Status: "pending"}, nil
 }
 
 type published struct{ actions []string }
@@ -65,7 +79,7 @@ var rels = releases{
 
 func setup(denyEnv string) (*fakestore.Store, deployments.DeploymentsUsecase) {
 	store := fakestore.New()
-	usecase := deployments.NewDeploymentsUsecase(store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{denyEnv}, &published{})
+	usecase := deployments.NewDeploymentsUsecase(store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{denyEnv: denyEnv}, &published{}, &requests{})
 	return store, usecase
 }
 
```

Create `app/deploy/usecases/deployments/approval_test.go`:

```go
package deployments_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

type gated struct {
	store    *fakestore.Store
	requests *requests
	events   *published
}

func newGated() gated {
	return gated{store: fakestore.New(), requests: &requests{}, events: &published{}}
}

func (g gated) as(gate authz.Gate) deployments.DeploymentsUsecase {
	return deployments.NewDeploymentsUsecase(g.store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{gate: gate}, g.events, g.requests)
}

func (g gated) live(t *testing.T, env, releaseID string) entities.Deployment {
	t.Helper()
	d, err := g.as(authz.Open).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: env, ReleaseID: releaseID})
	require.NoError(t, err)
	succeed(g.store, d)
	return d
}

func (g gated) proposal(t *testing.T) deployments.Proposal {
	t.Helper()
	raw, err := json.Marshal(g.requests.changes[len(g.requests.changes)-1].Proposal)
	require.NoError(t, err)
	var p deployments.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

var approvedAna = actor.Actor{UserID: "ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "mia"}}

func Test_Deploy_NeedingApprovalSubmitsInsteadOfScheduling(t *testing.T) {
	g := newGated()
	live := g.live(t, "prod", "r1")

	_, err := g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Len(t, g.store.ByID, 1)
	c := g.requests.changes[0]
	require.Equal(t, "deploy", c.Subject)
	require.Equal(t, "deploy release #2 to prod", c.Summary)
	require.Equal(t, deployments.Proposal{
		Kind: approval.KindDeploy, Service: "api", Environment: "prod", ReleaseID: "r2", Strategy: "bluegreen", LiveID: live.ID,
	}, g.proposal(t))
}

func Test_Promote_PinsTheReleaseThatWasLiveWhenAsked(t *testing.T) {
	g := newGated()
	g.live(t, "dev", "r1")
	_, _ = g.as(authz.NeedsApproval).Promote(ctx, ana, "api", "dev", "prod")
	g.live(t, "dev", "r2")

	d, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 1, d.ReleaseNumber)
	require.Equal(t, entities.TriggerPromote, d.Trigger)
	require.Equal(t, "promote release #1 to prod", g.requests.changes[0].Summary)
}

func Test_ApplyApproved_FailsWhenAnotherDeploymentWentLive(t *testing.T) {
	g := newGated()
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})
	g.live(t, "prod", "r1")

	_, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.Equal(t, apperror.CodeStale, apperror.CodeOf(err))
}

func Test_ApplyApproved_RollsBackToThePinnedDeployment(t *testing.T) {
	g := newGated()
	first := g.live(t, "prod", "r1")
	g.live(t, "prod", "r2")
	_, _ = g.as(authz.NeedsApproval).Rollback(ctx, ana, first.ID)

	d, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 1, d.ReleaseNumber)
	require.Equal(t, entities.TriggerRollback, d.Trigger)
}

func Test_Schedule_PublishesTheBypassWithTheDeployment(t *testing.T) {
	g := newGated()

	_, err := g.as(authz.Bypassed).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r1"})

	require.NoError(t, err)
	require.Equal(t, []string{"deploy.requested", "token.bypass_used"}, g.events.actions)
}

func Test_Preview_ShowsWhatIsLiveAgainstTheTarget(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})
	p := g.proposal(t)
	g.live(t, "prod", "r1")

	preview, err := g.as(authz.Open).Preview(ctx, p)

	require.NoError(t, err)
	require.Equal(t, approval.Line{Key: "release", Before: "#1", After: "#2"}, preview.Lines[0])
	require.Equal(t, approval.Line{Key: "config", Before: "#2", After: "#2"}, preview.Lines[1])
	require.Contains(t, preview.Conflict, "deployment #2 went live")
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./app/deploy/usecases/deployments/`
Expected: build failure: `deployments.Proposal` undefined; too many arguments to `NewDeploymentsUsecase`

- [ ] **Step 3: Implement**

Modify `app/deploy/usecases/deployments/deployments.go` (apply this diff; context lines are the current code):

```diff
--- a/app/deploy/usecases/deployments/deployments.go
+++ b/app/deploy/usecases/deployments/deployments.go
@@ -6,6 +6,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/app/deploy/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/events"
 )
 
@@ -37,7 +38,11 @@ type EnvironmentLookup interface {
 }
 
 type Authorizer interface {
-	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
+	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
+}
+
+type Requests interface {
+	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
 }
 
 type Publisher interface {
@@ -52,12 +57,13 @@ type DeploymentsUsecase struct {
 	environments EnvironmentLookup
 	authz        Authorizer
 	publisher    Publisher
+	requests     Requests
 }
 
-func NewDeploymentsUsecase(store Store, services ServiceLookup, releases ReleaseLookup, configs ConfigSource, environments EnvironmentLookup, authz Authorizer, publisher Publisher) DeploymentsUsecase {
+func NewDeploymentsUsecase(store Store, services ServiceLookup, releases ReleaseLookup, configs ConfigSource, environments EnvironmentLookup, authz Authorizer, publisher Publisher, requests Requests) DeploymentsUsecase {
 	return DeploymentsUsecase{
 		store: store, services: services, releases: releases, configs: configs,
-		environments: environments, authz: authz, publisher: publisher,
+		environments: environments, authz: authz, publisher: publisher, requests: requests,
 	}
 }
 
@@ -93,6 +99,6 @@ func publish(ctx context.Context, p Publisher, a actor.Actor, action string, ser
 	p.Publish(ctx, events.Event{
 		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetDeployment,
 		TargetID: service.ID, TargetName: service.Slug, TeamID: service.OwnerTeamID, Environment: d.Environment,
-		Details: map[string]any{"deployment": d.Number, "release": d.ReleaseNumber, "config": d.ConfigRevision, "trigger": string(d.Trigger)},
+		Details: events.WithApproval(a, map[string]any{"deployment": d.Number, "release": d.ReleaseNumber, "config": d.ConfigRevision, "trigger": string(d.Trigger)}),
 	})
 }
```

Modify `app/deploy/usecases/deployments/schedule.go` (apply this diff; context lines are the current code):

```diff
--- a/app/deploy/usecases/deployments/schedule.go
+++ b/app/deploy/usecases/deployments/schedule.go
@@ -20,6 +20,7 @@ var (
 
 type plan struct {
 	actor    actor.Actor
+	gate     authz.Gate
 	service  entities.ServiceRef
 	env      string
 	release  entities.ReleaseRef
@@ -28,16 +29,21 @@ type plan struct {
 	trigger  entities.Trigger
 }
 
-func (u DeploymentsUsecase) authorize(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.ServiceRef, error) {
+func (p plan) resource() authz.Resource {
+	return authz.Resource{TeamID: p.service.OwnerTeamID, Environment: p.env}
+}
+
+func (u DeploymentsUsecase) authorize(ctx context.Context, a actor.Actor, serviceSlug, environment string) (plan, error) {
 	service, err := u.services.FindService(ctx, serviceSlug)
 	if err != nil {
-		return entities.ServiceRef{}, err
+		return plan{}, err
 	}
 	if err := u.environments.EnvironmentExists(ctx, environment); err != nil {
-		return entities.ServiceRef{}, err
+		return plan{}, err
 	}
-	resource := authz.Resource{TeamID: service.OwnerTeamID, Environment: environment}
-	return service, u.authz.Can(ctx, a, authz.Deploy, resource)
+	p := plan{actor: a, service: service, env: environment}
+	p.gate, err = u.authz.Gate(ctx, a, authz.Deploy, p.resource())
+	return p, err
 }
 
 func (u DeploymentsUsecase) deployable(ctx context.Context, service entities.ServiceRef, releaseID string) (entities.ReleaseRef, error) {
@@ -54,6 +60,13 @@ func (u DeploymentsUsecase) deployable(ctx context.Context, service entities.Ser
 	return rel, nil
 }
 
+func (u DeploymentsUsecase) proceed(ctx context.Context, p plan, o Proposal) (entities.Deployment, error) {
+	if p.gate == authz.NeedsApproval {
+		return entities.Deployment{}, u.propose(ctx, p, o)
+	}
+	return u.schedule(ctx, p)
+}
+
 func (u DeploymentsUsecase) schedule(ctx context.Context, p plan) (entities.Deployment, error) {
 	if err := u.clearWay(ctx, p.service.ID, p.env); err != nil {
 		return entities.Deployment{}, err
@@ -68,6 +81,7 @@ func (u DeploymentsUsecase) schedule(ctx context.Context, p plan) (entities.Depl
 		return entities.Deployment{}, err
 	}
 	publish(ctx, u.publisher, p.actor, "deploy.requested", p.service, d)
+	authz.PublishBypass(ctx, u.publisher, p.gate, p.actor, authz.Deploy, p.resource())
 	return d, nil
 }
 
```

Modify `app/deploy/usecases/deployments/actions.go` (apply this diff; context lines are the current code):

```diff
--- a/app/deploy/usecases/deployments/actions.go
+++ b/app/deploy/usecases/deployments/actions.go
@@ -6,6 +6,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/deploy/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 )
 
 var (
@@ -27,57 +28,61 @@ func (u DeploymentsUsecase) Deploy(ctx context.Context, a actor.Actor, r Request
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	service, err := u.authorize(ctx, a, r.Service, r.Environment)
+	p, err := u.authorize(ctx, a, r.Service, r.Environment)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	rel, err := u.deployable(ctx, service, r.ReleaseID)
-	if err != nil {
+	if p.release, err = u.deployable(ctx, p.service, r.ReleaseID); err != nil {
 		return entities.Deployment{}, err
 	}
-	config, err := u.configs.CurrentNumber(ctx, service.Slug, r.Environment)
-	if err != nil {
+	if p.config, err = u.configs.CurrentNumber(ctx, p.service.Slug, p.env); err != nil {
 		return entities.Deployment{}, err
 	}
-	return u.schedule(ctx, plan{a, service, r.Environment, rel, config, strategy, entities.TriggerManual})
+	p.strategy, p.trigger = strategy, entities.TriggerManual
+	return u.proceed(ctx, p, Proposal{Kind: approval.KindDeploy, ReleaseID: p.release.ID, Strategy: string(strategy)})
 }
 
 func (u DeploymentsUsecase) ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error) {
-	service, err := u.authorize(ctx, a, serviceSlug, environment)
+	p, err := u.authorize(ctx, a, serviceSlug, environment)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	live, rel, err := u.live(ctx, service, environment)
+	live, rel, err := u.live(ctx, p.service, environment)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	config, err := u.configs.CurrentNumber(ctx, service.Slug, environment)
+	config, err := u.configs.CurrentNumber(ctx, p.service.Slug, environment)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
 	if config == live.ConfigRevision {
 		return entities.Deployment{}, errConfigIsLive
 	}
-	return u.schedule(ctx, plan{a, service, environment, rel, config, live.Strategy, entities.TriggerConfig})
+	p.release, p.config, p.strategy, p.trigger = rel, config, live.Strategy, entities.TriggerConfig
+	return u.proceed(ctx, p, Proposal{Kind: approval.KindConfigApply})
 }
 
 func (u DeploymentsUsecase) Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (entities.Deployment, error) {
 	if from == to {
 		return entities.Deployment{}, errSameEnvironment
 	}
-	service, err := u.authorize(ctx, a, serviceSlug, to)
+	p, err := u.authorize(ctx, a, serviceSlug, to)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	_, rel, err := u.live(ctx, service, from)
-	if err != nil {
+	if _, p.release, err = u.live(ctx, p.service, from); err != nil {
 		return entities.Deployment{}, err
 	}
-	config, err := u.configs.CurrentNumber(ctx, service.Slug, to)
+	return u.promote(ctx, p, from)
+}
+
+func (u DeploymentsUsecase) promote(ctx context.Context, p plan, from string) (entities.Deployment, error) {
+	config, err := u.configs.CurrentNumber(ctx, p.service.Slug, p.env)
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	return u.schedule(ctx, plan{a, service, to, rel, config, entities.StrategyBlueGreen, entities.TriggerPromote})
+	p.config, p.strategy, p.trigger = config, entities.StrategyBlueGreen, entities.TriggerPromote
+	return u.proceed(ctx, p, Proposal{Kind: approval.KindPromote, ReleaseID: p.release.ID, From: from})
 }
 
 func (u DeploymentsUsecase) Rollback(ctx context.Context, a actor.Actor, deploymentID string) (entities.Deployment, error) {
@@ -89,14 +94,16 @@ func (u DeploymentsUsecase) Rollback(ctx context.Context, a actor.Actor, deploym
 	if err != nil {
 		return entities.Deployment{}, err
 	}
-	if _, err := u.authorize(ctx, a, service.Slug, target.Environment); err != nil {
+	p, err := u.authorize(ctx, a, service.Slug, target.Environment)
+	if err != nil {
 		return entities.Deployment{}, err
 	}
 	if target.Status != entities.StatusSucceeded {
 		return entities.Deployment{}, errNotSucceeded
 	}
-	rel := entities.ReleaseRef{ID: target.ReleaseID, ServiceID: service.ID, Number: target.ReleaseNumber, Built: true, Reference: target.ImageRef}
-	return u.schedule(ctx, plan{a, service, target.Environment, rel, target.ConfigRevision, entities.StrategyBlueGreen, entities.TriggerRollback})
+	p.release = entities.ReleaseRef{ID: target.ReleaseID, ServiceID: service.ID, Number: target.ReleaseNumber, Built: true, Reference: target.ImageRef}
+	p.config, p.strategy, p.trigger = target.ConfigRevision, entities.StrategyBlueGreen, entities.TriggerRollback
+	return u.proceed(ctx, p, Proposal{Kind: approval.KindRollback, DeploymentID: target.ID})
 }
 
 func (u DeploymentsUsecase) live(ctx context.Context, service entities.ServiceRef, environment string) (entities.Deployment, entities.ReleaseRef, error) {
```

Create `app/deploy/usecases/deployments/approval.go`:

```go
package deployments

import (
	"context"
	"fmt"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind         string `json:"kind"`
	Service      string `json:"service"`
	Environment  string `json:"environment"`
	ReleaseID    string `json:"release_id,omitempty"`
	Strategy     string `json:"strategy,omitempty"`
	From         string `json:"from,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	LiveID       string `json:"live_id,omitempty"`
}

func (u DeploymentsUsecase) propose(ctx context.Context, p plan, o Proposal) error {
	live, _, err := u.store.LatestSucceeded(ctx, p.service.ID, p.env)
	if err != nil {
		return err
	}
	o.Service, o.Environment, o.LiveID = p.service.Slug, p.env, live.ID
	pending, err := u.requests.Submit(ctx, p.actor, approval.Change{
		Kind: o.Kind, Subject: "deploy", ServiceID: p.service.ID, ServiceSlug: p.service.Slug,
		TeamID: p.service.OwnerTeamID, Environment: p.env, Summary: summaryOf(o.Kind, p), Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func summaryOf(kind string, p plan) string {
	switch kind {
	case approval.KindPromote:
		return fmt.Sprintf("promote release #%d to %s", p.release.Number, p.env)
	case approval.KindRollback:
		return fmt.Sprintf("roll %s back to release #%d with config #%d", p.env, p.release.Number, p.config)
	case approval.KindConfigApply:
		return fmt.Sprintf("apply config #%d to %s", p.config, p.env)
	}
	return fmt.Sprintf("deploy release #%d to %s", p.release.Number, p.env)
}

func (u DeploymentsUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error) {
	if err := u.stillLive(ctx, o); err != nil {
		return entities.Deployment{}, err
	}
	switch o.Kind {
	case approval.KindPromote:
		return u.promotePinned(ctx, a, o)
	case approval.KindRollback:
		return u.Rollback(ctx, a, o.DeploymentID)
	case approval.KindConfigApply:
		return u.ApplyConfig(ctx, a, o.Service, o.Environment)
	}
	return u.Deploy(ctx, a, Request{Service: o.Service, Environment: o.Environment, ReleaseID: o.ReleaseID, Strategy: o.Strategy})
}

func (u DeploymentsUsecase) stillLive(ctx context.Context, o Proposal) error {
	live, _, err := u.Live(ctx, o.Service, o.Environment)
	if err != nil || live.ID == o.LiveID {
		return err
	}
	return apperror.Stale(fmt.Sprintf("deployment #%d went live after the request", live.Number))
}

func (u DeploymentsUsecase) promotePinned(ctx context.Context, a actor.Actor, o Proposal) (entities.Deployment, error) {
	p, err := u.authorize(ctx, a, o.Service, o.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if p.release, err = u.deployable(ctx, p.service, o.ReleaseID); err != nil {
		return entities.Deployment{}, err
	}
	return u.promote(ctx, p, o.From)
}

func (u DeploymentsUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	live, _, err := u.Live(ctx, o.Service, o.Environment)
	if err != nil {
		return approval.Preview{}, err
	}
	target, err := u.targetOf(ctx, o, live)
	if err != nil {
		return approval.Preview{}, err
	}
	preview := approval.Preview{Lines: []approval.Line{
		{Key: "release", Before: liveNumber(live, live.ReleaseNumber), After: fmt.Sprintf("#%d", target.ReleaseNumber)},
		{Key: "config", Before: liveNumber(live, live.ConfigRevision), After: fmt.Sprintf("#%d", target.ConfigRevision)},
	}}
	if live.ID != o.LiveID {
		preview.Conflict = fmt.Sprintf("approving will fail: deployment #%d went live after the request", live.Number)
	}
	return preview, nil
}

func (u DeploymentsUsecase) targetOf(ctx context.Context, o Proposal, live entities.Deployment) (entities.Deployment, error) {
	if o.Kind == approval.KindRollback {
		return u.store.FindByID(ctx, o.DeploymentID)
	}
	config, err := u.configs.CurrentNumber(ctx, o.Service, o.Environment)
	if err != nil || o.Kind == approval.KindConfigApply {
		return entities.Deployment{ReleaseNumber: live.ReleaseNumber, ConfigRevision: config}, err
	}
	rel, err := u.releases.FindRelease(ctx, o.ReleaseID)
	return entities.Deployment{ReleaseNumber: rel.Number, ConfigRevision: config}, err
}

func liveNumber(live entities.Deployment, number int) string {
	if live.ID == "" {
		return ""
	}
	return fmt.Sprintf("#%d", number)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/deploy/... && go vet ./...`
Expected: PASS

---

### Task 5: Rules — queue create, update and delete become requests

**Files:**
- Modify: `app/queue/usecases/queues/fakes_test.go`
- Create: `app/queue/usecases/queues/approval_test.go`
- Modify: `app/queue/usecases/queues/queues.go`
- Modify: `app/queue/usecases/queues/edit.go`
- Modify: `app/queue/usecases/queues/delete.go`
- Modify: `app/queue/usecases/queues/dead.go`
- Create: `app/queue/usecases/queues/approval.go`

`editable` returns the gate; `operable` (pause, resume, dead letters) turns `needs_approval` into `err_approval_required` with `authz.Direct`. `publish` takes the gate and publishes the bypass event after each queue write.

**Interfaces:**
- Consumes: Task 2 types.
- Produces:

```go
package queues
type Authorizer interface { Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error) }
type Requests interface { Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error) }
func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher, requests Requests) QueuesUsecase
type Proposal struct { Kind, Environment, Name string; Draft *Draft; Change *Change; Force bool }
func (u QueuesUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Queue, error)
func (u QueuesUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error)
```


- [ ] **Step 1: Write the failing tests**

Modify `app/queue/usecases/queues/fakes_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/queue/usecases/queues/fakes_test.go
+++ b/app/queue/usecases/queues/fakes_test.go
@@ -10,6 +10,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/events"
 )
 
@@ -127,13 +128,26 @@ func (s stats) Latest(id string) (entities.Stats, bool) {
 	return st, ok
 }
 
-type policy struct{ deny bool }
+type policy struct {
+	deny bool
+	gate authz.Gate
+}
 
-func (p policy) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error {
+func (p policy) Gate(context.Context, actor.Actor, authz.Action, authz.Resource) (authz.Gate, error) {
 	if p.deny {
-		return apperror.Forbidden("no")
+		return "", apperror.Forbidden("no")
 	}
-	return nil
+	if p.gate == "" {
+		return authz.Open, nil
+	}
+	return p.gate, nil
+}
+
+type requests struct{ changes []approval.Change }
+
+func (r *requests) Submit(_ context.Context, _ actor.Actor, c approval.Change) (*approval.Pending, error) {
+	r.changes = append(r.changes, c)
+	return &approval.Pending{ID: "c1", Status: "pending"}, nil
 }
 
 type recorder struct{ actions []string }
@@ -143,19 +157,27 @@ func (r *recorder) Publish(_ context.Context, e events.Event) {
 }
 
 type fixture struct {
-	usecase queues.QueuesUsecase
-	store   *store
-	stats   stats
-	events  *recorder
+	usecase  queues.QueuesUsecase
+	store    *store
+	stats    stats
+	events   *recorder
+	requests *requests
 }
 
 func newFixture(t *testing.T, deny bool) fixture {
 	t.Helper()
-	s := &store{queues: map[string]entities.Queue{}}
-	st := stats{}
-	r := &recorder{}
-	u := queues.NewQueuesUsecase(s, lookup{}, lookup{}, lookup{}, st, policy{deny: deny}, r)
-	return fixture{usecase: u, store: s, stats: st, events: r}
+	return fixtureWith(t, policy{deny: deny})
+}
+
+func fixtureWith(t *testing.T, p policy) fixture {
+	t.Helper()
+	f := fixture{store: &store{queues: map[string]entities.Queue{}}, stats: stats{}, events: &recorder{}, requests: &requests{}}
+	f.usecase = queues.NewQueuesUsecase(f.store, lookup{}, lookup{}, lookup{}, f.stats, p, f.events, f.requests)
+	return f
+}
+
+func (f fixture) as(gate authz.Gate) queues.QueuesUsecase {
+	return queues.NewQueuesUsecase(f.store, lookup{}, lookup{}, lookup{}, f.stats, policy{gate: gate}, f.events, f.requests)
 }
 
 func draft() queues.Draft {
```

Create `app/queue/usecases/queues/approval_test.go`:

```go
package queues_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

var approvedAna = actor.Actor{UserID: "u-ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "mia"}}

func (f fixture) proposal(t *testing.T) queues.Proposal {
	t.Helper()
	raw, err := json.Marshal(f.requests.changes[len(f.requests.changes)-1].Proposal)
	require.NoError(t, err)
	var p queues.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

func Test_Create_NeedingApprovalSubmitsTheDraft(t *testing.T) {
	f := newFixture(t, false)

	_, err := f.as(authz.NeedsApproval).Create(ctx, ana, draft())

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Empty(t, f.store.queues)
	c := f.requests.changes[0]
	require.Equal(t, "queue.place-import", c.Subject)
	require.Equal(t, "create queue place-import", c.Summary)
	require.Equal(t, "worker", c.ServiceSlug)
	created, err := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))
	require.NoError(t, err)
	require.Equal(t, 1, created.Version)
}

func Test_Update_NeedingApprovalKeepsTheBaseVersion(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	settings := created.Settings
	settings.MaxAttempts = 9

	_, err := f.as(authz.NeedsApproval).Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &settings, BaseVersion: 1})
	preview, _ := f.usecase.Preview(ctx, f.proposal(t))
	_, _ = f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &created.Settings, BaseVersion: 1})
	_, stale := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))

	require.Error(t, err)
	require.Equal(t, []approval.Line{{Key: "max_attempts", Before: "5", After: "9"}}, preview.Lines)
	require.ErrorIs(t, stale, apperror.Conflict(""))
}

func Test_Delete_NeedingApprovalStillRefusesAQueueWithMessages(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[created.ID] = fresh(3)

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "place-import", false)

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.Empty(t, f.requests.changes)
}

func Test_PauseAndDeadLettersStayRefusedWithoutBypass(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, paused := f.as(authz.NeedsApproval).SetPaused(ctx, ana, "prod", "place-import", true)
	purged := f.as(authz.NeedsApproval).PurgeDead(ctx, ana, "prod", "place-import")

	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(paused))
	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(purged))
	require.Empty(t, f.requests.changes)
}

func Test_QueueEdits_PublishTheBypassWhenMade(t *testing.T) {
	f := newFixture(t, false)

	_, err := f.as(authz.Bypassed).Create(ctx, ana, draft())

	require.NoError(t, err)
	require.Equal(t, []string{"queue.created", "token.bypass_used"}, f.events.actions)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./app/queue/usecases/queues/`
Expected: build failure: `queues.Proposal` undefined; too many arguments to `NewQueuesUsecase`

- [ ] **Step 3: Implement**

Modify `app/queue/usecases/queues/queues.go` (apply this diff; context lines are the current code):

```diff
--- a/app/queue/usecases/queues/queues.go
+++ b/app/queue/usecases/queues/queues.go
@@ -6,6 +6,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/app/queue/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/events"
 )
 
@@ -45,7 +46,11 @@ type StatsReader interface {
 }
 
 type Authorizer interface {
-	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
+	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
+}
+
+type Requests interface {
+	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
 }
 
 type Publisher interface {
@@ -60,12 +65,13 @@ type QueuesUsecase struct {
 	stats        StatsReader
 	authz        Authorizer
 	publisher    Publisher
+	requests     Requests
 }
 
-func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher) QueuesUsecase {
+func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher, requests Requests) QueuesUsecase {
 	return QueuesUsecase{
 		store: store, services: services, environments: environments, teams: teams,
-		stats: stats, authz: authz, publisher: publisher,
+		stats: stats, authz: authz, publisher: publisher, requests: requests,
 	}
 }
 
@@ -101,17 +107,29 @@ func (u QueuesUsecase) Stats(ctx context.Context, environment, name string) (ent
 	return st, ok, nil
 }
 
-func (u QueuesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error) {
+func (u QueuesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, authz.Gate, error) {
 	q, err := u.store.Get(ctx, environment, name)
 	if err != nil {
-		return entities.Queue{}, err
+		return entities.Queue{}, "", err
 	}
-	return q, u.authz.Can(ctx, a, authz.EditQueue, authz.Resource{TeamID: q.TeamID, Environment: q.Environment})
+	gate, err := u.authz.Gate(ctx, a, authz.EditQueue, resourceOf(q))
+	return q, gate, err
+}
+
+func (u QueuesUsecase) operable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, authz.Gate, error) {
+	q, gate, err := u.editable(ctx, a, environment, name)
+	gate, err = authz.Direct(gate, err)
+	return q, gate, err
+}
+
+func resourceOf(q entities.Queue) authz.Resource {
+	return authz.Resource{TeamID: q.TeamID, Environment: q.Environment}
 }
 
-func (u QueuesUsecase) publish(ctx context.Context, a actor.Actor, action string, q entities.Queue, details map[string]any) {
+func (u QueuesUsecase) publish(ctx context.Context, a actor.Actor, gate authz.Gate, action string, q entities.Queue, details map[string]any) {
 	u.publisher.Publish(ctx, events.Event{
 		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetQueue,
-		TargetID: q.ID, TargetName: q.Name, TeamID: q.TeamID, Environment: q.Environment, Details: details,
+		TargetID: q.ID, TargetName: q.Name, TeamID: q.TeamID, Environment: q.Environment, Details: events.WithApproval(a, details),
 	})
+	authz.PublishBypass(ctx, u.publisher, gate, a, authz.EditQueue, resourceOf(q))
 }
```

Modify `app/queue/usecases/queues/edit.go` (apply this diff; context lines are the current code):

```diff
--- a/app/queue/usecases/queues/edit.go
+++ b/app/queue/usecases/queues/edit.go
@@ -8,6 +8,7 @@ import (
 	"github.com/klzn-labs/kalzone/app/queue/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/clock"
 	"github.com/klzn-labs/kalzone/pkg/uuid"
 )
@@ -35,17 +36,25 @@ func (u QueuesUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (enti
 	if err != nil {
 		return entities.Queue{}, err
 	}
-	if err := u.authz.Can(ctx, a, authz.EditQueue, authz.Resource{TeamID: consumer.OwnerTeamID, Environment: d.Environment}); err != nil {
+	gate, err := u.authz.Gate(ctx, a, authz.EditQueue, authz.Resource{TeamID: consumer.OwnerTeamID, Environment: d.Environment})
+	if err != nil {
 		return entities.Queue{}, err
 	}
 	q, err := u.newQueue(ctx, d, consumer)
 	if err != nil {
 		return entities.Queue{}, err
 	}
-	if err := u.store.Create(ctx, q, versionOf(a, q, d.Message)); err != nil {
+	if gate == authz.NeedsApproval {
+		return entities.Queue{}, u.propose(ctx, a, q, Proposal{Kind: approval.KindQueueCreate, Draft: &d})
+	}
+	return u.create(ctx, a, gate, q, d.Message)
+}
+
+func (u QueuesUsecase) create(ctx context.Context, a actor.Actor, gate authz.Gate, q entities.Queue, message string) (entities.Queue, error) {
+	if err := u.store.Create(ctx, q, versionOf(a, q, message)); err != nil {
 		return entities.Queue{}, err
 	}
-	u.publish(ctx, a, "queue.created", q, map[string]any{"version": q.Version})
+	u.publish(ctx, a, gate, "queue.created", q, map[string]any{"version": q.Version})
 	return q, nil
 }
 
@@ -94,7 +103,7 @@ func (u QueuesUsecase) publisherIDs(ctx context.Context, consumer ServiceRef, sl
 }
 
 func (u QueuesUsecase) Update(ctx context.Context, a actor.Actor, environment, name string, c Change) (entities.Queue, error) {
-	q, err := u.editable(ctx, a, environment, name)
+	q, gate, err := u.editable(ctx, a, environment, name)
 	if err != nil {
 		return entities.Queue{}, err
 	}
@@ -102,10 +111,13 @@ func (u QueuesUsecase) Update(ctx context.Context, a actor.Actor, environment, n
 	if err != nil {
 		return entities.Queue{}, err
 	}
+	if gate == authz.NeedsApproval {
+		return entities.Queue{}, u.propose(ctx, a, q, Proposal{Kind: approval.KindQueueUpdate, Change: &c})
+	}
 	if err := u.store.InsertVersion(ctx, next, versionOf(a, next, c.Message)); err != nil {
 		return entities.Queue{}, err
 	}
-	u.publish(ctx, a, "queue.updated", next, map[string]any{"version": next.Version, "message": c.Message})
+	u.publish(ctx, a, gate, "queue.updated", next, map[string]any{"version": next.Version, "message": c.Message})
 	return next, nil
 }
 
```

Modify `app/queue/usecases/queues/delete.go` (apply this diff; context lines are the current code):

```diff
--- a/app/queue/usecases/queues/delete.go
+++ b/app/queue/usecases/queues/delete.go
@@ -3,9 +3,11 @@ package queues
 import (
 	"context"
 
+	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/app/queue/entities"
 	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/klzn-labs/kalzone/pkg/clock"
 	"github.com/klzn-labs/kalzone/pkg/uuid"
 )
@@ -13,7 +15,7 @@ import (
 var errNotEmpty = apperror.Conflict("the queue still holds messages; empty it or delete with force")
 
 func (u QueuesUsecase) SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error) {
-	q, err := u.editable(ctx, a, environment, name)
+	q, gate, err := u.operable(ctx, a, environment, name)
 	if err != nil {
 		return entities.Queue{}, err
 	}
@@ -21,22 +23,25 @@ func (u QueuesUsecase) SetPaused(ctx context.Context, a actor.Actor, environment
 		return entities.Queue{}, err
 	}
 	q.Paused = paused
-	u.publish(ctx, a, map[bool]string{true: "queue.paused", false: "queue.resumed"}[paused], q, nil)
+	u.publish(ctx, a, gate, map[bool]string{true: "queue.paused", false: "queue.resumed"}[paused], q, nil)
 	return q, nil
 }
 
 func (u QueuesUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error {
-	q, err := u.editable(ctx, a, environment, name)
+	q, gate, err := u.editable(ctx, a, environment, name)
 	if err != nil {
 		return err
 	}
 	if !force && !u.knownEmpty(q.ID) {
 		return errNotEmpty
 	}
+	if gate == authz.NeedsApproval {
+		return u.propose(ctx, a, q, Proposal{Kind: approval.KindQueueDelete, Force: force})
+	}
 	if err := u.store.Delete(ctx, q.ID, newCommand(q, entities.CommandDelete, q.Environment+"/"+q.Name)); err != nil {
 		return err
 	}
-	u.publish(ctx, a, "queue.deleted", q, map[string]any{"forced": force})
+	u.publish(ctx, a, gate, "queue.deleted", q, map[string]any{"forced": force})
 	return nil
 }
 
```

Modify `app/queue/usecases/queues/dead.go` (apply this diff; context lines are the current code):

```diff
--- a/app/queue/usecases/queues/dead.go
+++ b/app/queue/usecases/queues/dead.go
@@ -20,13 +20,13 @@ func (u QueuesUsecase) PurgeDead(ctx context.Context, a actor.Actor, environment
 }
 
 func (u QueuesUsecase) deadCommand(ctx context.Context, a actor.Actor, environment, name, kind, argument, action string) error {
-	q, err := u.editable(ctx, a, environment, name)
+	q, gate, err := u.operable(ctx, a, environment, name)
 	if err != nil {
 		return err
 	}
 	if err := u.command(ctx, q, kind, argument); err != nil {
 		return err
 	}
-	u.publish(ctx, a, action, q, map[string]any{"message": argument})
+	u.publish(ctx, a, gate, action, q, map[string]any{"message": argument})
 	return nil
 }
```

Create `app/queue/usecases/queues/approval.go`:

```go
package queues

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind        string  `json:"kind"`
	Environment string  `json:"environment"`
	Name        string  `json:"name"`
	Draft       *Draft  `json:"draft,omitempty"`
	Change      *Change `json:"change,omitempty"`
	Force       bool    `json:"force,omitempty"`
}

var (
	summaries          = map[string]string{approval.KindQueueCreate: "create queue ", approval.KindQueueUpdate: "change queue ", approval.KindQueueDelete: "delete queue "}
	errUnknownProposal = apperror.Invalid("this request cannot be executed")
)

func (u QueuesUsecase) propose(ctx context.Context, a actor.Actor, q entities.Queue, o Proposal) error {
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if err != nil {
		return err
	}
	o.Environment, o.Name = q.Environment, q.Name
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: o.Kind, Subject: "queue." + q.Name, ServiceID: q.ConsumerID, ServiceSlug: consumer.Slug,
		TeamID: q.TeamID, Environment: q.Environment, Summary: summaries[o.Kind] + q.Name, Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func (u QueuesUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Queue, error) {
	switch {
	case o.Kind == approval.KindQueueCreate && o.Draft != nil:
		return u.Create(ctx, a, *o.Draft)
	case o.Kind == approval.KindQueueUpdate && o.Change != nil:
		return u.Update(ctx, a, o.Environment, o.Name, *o.Change)
	case o.Kind == approval.KindQueueDelete:
		return entities.Queue{}, u.Delete(ctx, a, o.Environment, o.Name, o.Force)
	}
	return entities.Queue{}, errUnknownProposal
}

func (u QueuesUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	switch {
	case o.Kind == approval.KindQueueCreate && o.Draft != nil:
		return approval.Preview{Lines: settingsLines(nil, settingFields(withDefaults(o.Draft.Settings)))}, nil
	case o.Kind == approval.KindQueueUpdate && o.Change != nil:
		return u.previewUpdate(ctx, o)
	}
	return approval.Preview{Lines: []approval.Line{{Key: "queue", Before: o.Name, After: "deleted"}}}, nil
}

func (u QueuesUsecase) previewUpdate(ctx context.Context, o Proposal) (approval.Preview, error) {
	q, err := u.store.Get(ctx, o.Environment, o.Name)
	if err != nil {
		return approval.Preview{}, err
	}
	after := q.Settings
	if o.Change.Settings != nil {
		after = *o.Change.Settings
	}
	preview := approval.Preview{Lines: settingsLines(settingFields(q.Settings), settingFields(after))}
	if o.Change.Publishers != nil {
		preview.Lines = append(preview.Lines, approval.Line{Key: "publishers", After: strings.Join(*o.Change.Publishers, ", ")})
	}
	if q.Version != o.Change.BaseVersion {
		preview.Conflict = fmt.Sprintf("approving will fail: the queue moved from version %d to %d since the request", o.Change.BaseVersion, q.Version)
	}
	return preview, nil
}

func settingFields(s entities.Settings) [][2]string {
	return [][2]string{
		{"path", s.Path}, {"rate_per_second", strconv.FormatFloat(s.RatePerSecond, 'g', -1, 64)},
		{"max_in_flight", strconv.Itoa(s.MaxInFlight)}, {"max_attempts", strconv.Itoa(s.MaxAttempts)},
		{"backoff", fmt.Sprintf("%s %s to %s", s.Backoff.Kind, s.Backoff.Initial, s.Backoff.Max)},
		{"delivery_timeout", s.DeliveryTimeout.String()}, {"retention", s.Retention.String()},
		{"max_message_bytes", strconv.Itoa(s.MaxMessageBytes)},
	}
}

func settingsLines(before, after [][2]string) []approval.Line {
	lines := []approval.Line{}
	for i, field := range after {
		old := ""
		if before != nil {
			old = before[i][1]
		}
		if old != field[1] {
			lines = append(lines, approval.Line{Key: field[0], Before: old, After: field[1]})
		}
	}
	return lines
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/queue/... && go vet ./...`
Expected: PASS, the existing stale-version test still expects `err_conflict`

---

### Task 6: Rules — submit, approve, reject, cancel, sweep

**Files:**
- Modify: `app/identity/usecases/tokens/authenticate_test.go`
- Modify: `app/identity/usecases/tokens/authenticate.go`
- Create: `app/approvals/entities/event.go`
- Create: `app/approvals/usecases/submit/submit_test.go`
- Create: `app/approvals/usecases/submit/submit.go`
- Create: `app/approvals/usecases/requests/fakes_test.go`
- Create: `app/approvals/usecases/requests/requests_test.go`
- Create: `app/approvals/usecases/requests/requests.go`
- Create: `app/approvals/usecases/requests/decide.go`
- Create: `app/approvals/usecases/requests/sweep.go`

`submit` is what the domains call; it depends only on storage, the bus and the notifier. `requests` depends on the domains through executors, so the fx graph has no cycle. Approval rebuilds the token's actor with `TokensUsecase.ActorOf` and adds the approval before running the executor; any error becomes a `failed` request with its code, not an HTTP error. After this task the rules layer is complete; integration tests stay red until Task 7 provides the `Requests` ports to fx.

**Interfaces:**
- Consumes: Task 1 repository and entities, Task 2 types, the domain proposals of Tasks 3–5 (only through `requests.Executor`).
- Produces:

```go
package tokens
func (u TokensUsecase) ActorOf(ctx context.Context, tokenID string) (actor.Actor, error)

package entities
func (r Request) ApprovePath() string
func (r Request) PollPath() string
func EventOf(a actor.Actor, action string, r Request) events.Event

package submit
type Notifier interface { Notify(text string) }
func NewSubmitUsecase(cfg config.Config, store Store, notifier Notifier, publisher Publisher) SubmitUsecase
func (u SubmitUsecase) Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)

package requests
type Executor interface { Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error); Preview(ctx context.Context, payload []byte) (approval.Preview, error) }
type Executors map[string]Executor
type TokenActors interface { ActorOf(ctx context.Context, tokenID string) (actor.Actor, error) }
func NewRequestsUsecase(store Store, authz Authorizer, tokens TokenActors, executors Executors, publisher Publisher) RequestsUsecase
func (u RequestsUsecase) Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
func (u RequestsUsecase) List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error)
func (u RequestsUsecase) Awaiting(ctx context.Context, a actor.Actor) ([]entities.Request, error)
func (u RequestsUsecase) MayDecide(ctx context.Context, a actor.Actor, r entities.Request) bool
func (u RequestsUsecase) Preview(ctx context.Context, r entities.Request) (approval.Preview, error)
func (u RequestsUsecase) Approve(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
func (u RequestsUsecase) Reject(ctx context.Context, a actor.Actor, id, reason string) (entities.Request, error)
func (u RequestsUsecase) Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
func (u RequestsUsecase) Sweep(ctx context.Context) (int, error)
```


- [ ] **Step 1: Write the failing tests**

Modify `app/identity/usecases/tokens/authenticate_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/identity/usecases/tokens/authenticate_test.go
+++ b/app/identity/usecases/tokens/authenticate_test.go
@@ -56,3 +56,21 @@ func Test_Authenticate_TouchesAtMostOncePerMinute(t *testing.T) {
 	require.Equal(t, 3, f.store.touched)
 	require.Equal(t, "10.0.0.2", f.store.byID[issued.Token.ID].LastUsedIP)
 }
+
+func Test_ActorOf_RebuildsTheTokensActorWhileItIsValid(t *testing.T) {
+	f := newFixture()
+	issued, _ := f.usecase.Create(ctx, ana, draftOf("deploy", "protected"))
+	revoked, _ := f.usecase.Create(ctx, ana, lasting(1, entities.TokenDraft{Name: "gone", Scopes: []string{"read"}, Teams: []string{"places"}}))
+	require.NoError(t, f.usecase.Revoke(ctx, ana, revoked.Token.ID))
+
+	a, err := f.usecase.ActorOf(ctx, issued.Token.ID)
+	_, gone := f.usecase.ActorOf(ctx, revoked.Token.ID)
+	clock.WithTime(now.Add(91 * 24 * time.Hour))
+	_, late := f.usecase.ActorOf(ctx, issued.Token.ID)
+
+	require.NoError(t, err)
+	require.Equal(t, "u-ana", a.UserID)
+	require.Equal(t, issued.Token.ID, a.Token.ID)
+	require.ErrorIs(t, gone, entities.ErrTokenRevoked)
+	require.ErrorIs(t, late, entities.ErrTokenExpired)
+}
```

Create `app/approvals/usecases/submit/submit_test.go`:

```go
package submit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/submit"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	bob = actor.Actor{UserID: "u-bob", Login: "bob", Token: actor.Token{ID: "k1", Name: "claude-local"}}
)

type store struct{ byID map[string]entities.Request }

func (s *store) Create(_ context.Context, r entities.Request) error {
	s.byID[r.ID] = r
	return nil
}

func (s *store) PendingFor(_ context.Context, serviceID, environment, subject string) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if r.ServiceID == serviceID && r.Environment == environment && r.Subject == subject && r.Pending() {
			found = append(found, r)
		}
	}
	return found, nil
}

func (s *store) Transition(_ context.Context, r entities.Request, from entities.Status) error {
	if s.byID[r.ID].Status != from {
		return apperror.Conflict("changed")
	}
	s.byID[r.ID] = r
	return nil
}

type notes struct{ texts []string }

func (n *notes) Notify(text string) { n.texts = append(n.texts, text) }

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	store   *store
	notes   *notes
	events  *recorder
	usecase submit.SubmitUsecase
}

func newFixture() fixture {
	clock.WithTime(now)
	f := fixture{store: &store{byID: map[string]entities.Request{}}, notes: &notes{}, events: &recorder{}}
	f.usecase = submit.NewSubmitUsecase(config.Config{BaseURL: "http://platform.test/"}, f.store, f.notes, f.events)
	return f
}

func change(subject string) approval.Change {
	return approval.Change{
		Kind: approval.KindDeploy, Subject: subject, ServiceID: "s1", ServiceSlug: "api", TeamID: "t1",
		Environment: "prod", Summary: "deploy release #2 to prod", Proposal: map[string]string{"release_id": "r2"},
	}
}

func Test_Submit_StoresAPendingRequestAndSaysWhereToApproveIt(t *testing.T) {
	f := newFixture()

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	stored := f.store.byID[pending.ID]
	require.Equal(t, "change_request", pending.Kind)
	require.Equal(t, "http://platform.test/requests/"+pending.ID, pending.ApproveURL)
	require.Equal(t, "/api/v1/requests/"+pending.ID, pending.Poll)
	require.Equal(t, now.Add(24*time.Hour), pending.ExpiresAt)
	require.Equal(t, entities.StatusPending, stored.Status)
	require.Equal(t, "claude-local", stored.TokenName)
	require.JSONEq(t, `{"release_id":"r2"}`, string(stored.Payload))
	require.Equal(t, []string{"request.created"}, f.events.actions)
	require.Contains(t, f.notes.texts[0], "bob via claude-local asks to deploy release #2 to prod on api in prod")
	require.Contains(t, f.notes.texts[0], pending.ApproveURL)
}

func Test_Submit_SupersedesOlderPendingRequestsForTheSameSubject(t *testing.T) {
	f := newFixture()
	first, _ := f.usecase.Submit(ctx, bob, change("deploy"))
	other, _ := f.usecase.Submit(ctx, bob, change("secret.TOKEN"))

	second, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	older := f.store.byID[first.ID]
	require.Equal(t, entities.StatusCancelled, older.Status)
	require.Equal(t, "superseded by "+second.ID, older.Reason)
	require.Equal(t, entities.StatusPending, f.store.byID[other.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[second.ID].Status)
	require.Contains(t, f.events.actions, "request.superseded")
}

func Test_Submit_ClipsALongSummary(t *testing.T) {
	f := newFixture()
	long := change("config")
	long.Summary = "change " + strings.Repeat("env.KEY, ", 60)

	pending, err := f.usecase.Submit(ctx, bob, long)

	require.NoError(t, err)
	require.Len(t, []rune(f.store.byID[pending.ID].Summary), entities.SummaryLimit)
}
```

Create `app/approvals/usecases/requests/fakes_test.go`:

```go
package requests_test

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	mia = actor.Actor{UserID: "u-mia", Login: "mia"}
	bob = actor.Actor{UserID: "u-bob", Login: "bob"}
)

type store struct{ byID map[string]entities.Request }

func (s *store) Get(_ context.Context, id string) (entities.Request, error) {
	r, ok := s.byID[id]
	if !ok {
		return entities.Request{}, apperror.NotFound("change request not found")
	}
	return r, nil
}

func (s *store) List(_ context.Context, f entities.Filter) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if f.Status == "" || r.Status == f.Status {
			found = append(found, r)
		}
	}
	return found, nil
}

func (s *store) Overdue(_ context.Context, at time.Time) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if r.ExpiredAt(at) || (r.Status == entities.StatusApproved && !r.DecidedAt.After(at.Add(-entities.ExecutionLimit))) {
			found = append(found, r)
		}
	}
	return found, nil
}

func (s *store) Transition(_ context.Context, r entities.Request, from entities.Status) error {
	if s.byID[r.ID].Status != from {
		return apperror.Conflict("this request was already decided")
	}
	s.byID[r.ID] = r
	return nil
}

type maintainers map[string]string

func (m maintainers) Can(_ context.Context, a actor.Actor, _ authz.Action, r authz.Resource) error {
	if a.ViaToken() || m[a.UserID] != r.TeamID {
		return apperror.Forbidden("no")
	}
	return nil
}

type tokens struct{ err error }

func (t tokens) ActorOf(_ context.Context, tokenID string) (actor.Actor, error) {
	return actor.Actor{UserID: "u-bob", Login: "bob", Token: actor.Token{ID: tokenID, Name: "claude-local"}}, t.err
}

type executor struct {
	ran    []actor.Actor
	result string
	err    error
}

func (e *executor) Execute(_ context.Context, a actor.Actor, _ []byte) (string, error) {
	e.ran = append(e.ran, a)
	return e.result, e.err
}

func (e *executor) Preview(context.Context, []byte) (approval.Preview, error) {
	return approval.Preview{Lines: []approval.Line{{Key: "release", Before: "#1", After: "#2"}}}, nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	store    *store
	executor *executor
	events   *recorder
	tokens   *tokens
}

func newFixture() fixture {
	clock.WithTime(now)
	f := fixture{store: &store{byID: map[string]entities.Request{}}, executor: &executor{result: "deployment #3"}, events: &recorder{}, tokens: &tokens{}}
	f.store.byID["c1"] = entities.Request{
		ID: "c1", Kind: approval.KindDeploy, TeamID: "t1", RequestedBy: "u-bob", TokenID: "k1",
		Status: entities.StatusPending, CreatedAt: now, ExpiresAt: now.Add(entities.Lifetime),
	}
	return f
}

func (f fixture) usecase() requests.RequestsUsecase {
	return requests.NewRequestsUsecase(f.store, maintainers{"u-mia": "t1"}, f.tokens, requests.Executors{approval.KindDeploy: f.executor}, f.events)
}
```

Create `app/approvals/usecases/requests/requests_test.go`:

```go
package requests_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

func Test_Approve_RunsTheChangeAsTheTokenCarryingTheApproval(t *testing.T) {
	f := newFixture()

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, entities.StatusExecuted, r.Status)
	require.Equal(t, "deployment #3", f.store.byID["c1"].Result)
	ran := f.executor.ran[0]
	require.Equal(t, "u-bob", ran.UserID)
	require.Equal(t, "k1", ran.Token.ID)
	require.Equal(t, actor.Approval{RequestID: "c1", ApproverID: "u-mia", ApproverLogin: "mia"}, ran.Approval)
	require.Equal(t, []string{"request.approved", "request.executed"}, f.events.actions)
}

func Test_Approve_RecordsAFailureWithItsCode(t *testing.T) {
	f := newFixture()
	f.executor.err = apperror.Stale("env.A changed since the request")

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, entities.StatusFailed, r.Status)
	require.Equal(t, "err_stale", r.ErrorCode)
	require.Equal(t, "env.A changed since the request", f.store.byID["c1"].ErrorMessage)
	require.Equal(t, []string{"request.approved", "request.failed"}, f.events.actions)
}

func Test_Approve_FailsWhenTheTokenNoLongerWorks(t *testing.T) {
	f := newFixture()
	f.tokens.err = apperror.New(apperror.CodeTokenRevoked, "this access token was revoked")

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, "err_token_revoked", r.ErrorCode)
	require.Empty(t, f.executor.ran)
}

func Test_Approve_OnlyByAnApproverWhileStillPending(t *testing.T) {
	f := newFixture()

	_, notApprover := f.usecase().Approve(ctx, bob, "c1")
	clock.WithTime(now.Add(entities.Lifetime))
	_, expired := f.usecase().Approve(ctx, mia, "c1")
	clock.WithTime(now)
	_, _ = f.usecase().Approve(ctx, mia, "c1")
	_, twice := f.usecase().Approve(ctx, mia, "c1")

	require.ErrorIs(t, notApprover, apperror.Forbidden(""))
	require.ErrorIs(t, expired, apperror.Conflict(""))
	require.ErrorIs(t, twice, apperror.Conflict(""))
	require.Len(t, f.executor.ran, 1)
}

func Test_Reject_KeepsTheReason(t *testing.T) {
	f := newFixture()

	r, err := f.usecase().Reject(ctx, mia, "c1", "  not during the sale  ")

	require.NoError(t, err)
	require.Equal(t, entities.StatusRejected, r.Status)
	require.Equal(t, "not during the sale", f.store.byID["c1"].Reason)
	require.Empty(t, f.executor.ran)
}

func Test_Cancel_OnlyByThePersonWhoAsked(t *testing.T) {
	f := newFixture()

	_, notTheirs := f.usecase().Cancel(ctx, mia, "c1")
	r, err := f.usecase().Cancel(ctx, bob, "c1")

	require.ErrorIs(t, notTheirs, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, entities.StatusCancelled, r.Status)
	require.Equal(t, "cancelled by bob", r.Reason)
}

func Test_Get_HidesOtherTeamsFromATokenAndAwaitingListsWhatTheViewerDecides(t *testing.T) {
	f := newFixture()
	agent := actor.Actor{UserID: "u-bob", Token: actor.Token{ID: "k2", TeamIDs: []string{"t2"}}}

	_, hidden := f.usecase().Get(ctx, agent, "c1")
	forMia, _ := f.usecase().Awaiting(ctx, mia)
	forBob, _ := f.usecase().Awaiting(ctx, bob)

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.Len(t, forMia, 1)
	require.Empty(t, forBob)
}

func Test_Sweep_ExpiresPendingAndFailsStuckApprovals(t *testing.T) {
	f := newFixture()
	decided := now.Add(-time.Hour)
	f.store.byID["c2"] = entities.Request{ID: "c2", Status: entities.StatusApproved, DecidedAt: &decided, ExpiresAt: now.Add(time.Hour)}
	clock.WithTime(now.Add(entities.Lifetime))

	swept, err := f.usecase().Sweep(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, swept)
	require.Equal(t, entities.StatusExpired, f.store.byID["c1"].Status)
	require.Equal(t, "platform", f.store.byID["c1"].DeciderLogin)
	require.Equal(t, entities.StatusFailed, f.store.byID["c2"].Status)
	require.ElementsMatch(t, []string{"request.expired", "request.failed"}, f.events.actions)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./app/identity/usecases/tokens/ ./app/approvals/usecases/...`
Expected: build failure: `ActorOf`, `submit.NewSubmitUsecase`, `requests.NewRequestsUsecase` undefined

- [ ] **Step 3: Implement**

Modify `app/identity/usecases/tokens/authenticate.go` (apply this diff; context lines are the current code):

```diff
--- a/app/identity/usecases/tokens/authenticate.go
+++ b/app/identity/usecases/tokens/authenticate.go
@@ -5,6 +5,7 @@ import (
 	"time"
 
 	"github.com/klzn-labs/kalzone/app/identity/entities"
+	"github.com/klzn-labs/kalzone/pkg/actor"
 	"github.com/klzn-labs/kalzone/pkg/clock"
 )
 
@@ -35,3 +36,18 @@ func (u TokensUsecase) touchIfDue(ctx context.Context, t entities.AccessToken, n
 	}
 	return u.store.Touch(ctx, t.ID, now, ip)
 }
+
+func (u TokensUsecase) ActorOf(ctx context.Context, tokenID string) (actor.Actor, error) {
+	t, err := u.store.FindByID(ctx, tokenID)
+	if err != nil {
+		return actor.Actor{}, err
+	}
+	if err := t.Check(clock.Now().UTC()); err != nil {
+		return actor.Actor{}, err
+	}
+	user, err := u.users.FindByID(ctx, t.OwnerID)
+	if err != nil {
+		return actor.Actor{}, err
+	}
+	return entities.TokenAuthenticated{User: user, Token: t}.Actor(), nil
+}
```

Create `app/approvals/entities/event.go`:

```go
package entities

import (
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/events"
)

func (r Request) ApprovePath() string {
	return "/requests/" + r.ID
}

func (r Request) PollPath() string {
	return "/api/v1/requests/" + r.ID
}

func EventOf(a actor.Actor, action string, r Request) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetRequest, TargetID: r.ID, TargetName: r.ServiceSlug, TeamID: r.TeamID, Environment: r.Environment,
		Details: map[string]any{"kind": r.Kind, "summary": r.Summary, "reason": r.Reason, "result": r.Result, "error": r.ErrorMessage},
	}
}
```

Create `app/approvals/usecases/submit/submit.go`:

```go
package submit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type Store interface {
	Create(ctx context.Context, r entities.Request) error
	PendingFor(ctx context.Context, serviceID, environment, subject string) ([]entities.Request, error)
	Transition(ctx context.Context, r entities.Request, from entities.Status) error
}

type Notifier interface {
	Notify(text string)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type SubmitUsecase struct {
	store     Store
	notifier  Notifier
	publisher Publisher
	baseURL   string
}

func NewSubmitUsecase(cfg config.Config, store Store, notifier Notifier, publisher Publisher) SubmitUsecase {
	return SubmitUsecase{store: store, notifier: notifier, publisher: publisher, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
}

func (u SubmitUsecase) Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error) {
	r, err := newRequest(a, c)
	if err != nil {
		return nil, err
	}
	if err := u.store.Create(ctx, r); err != nil {
		return nil, err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, "request.created", r))
	if err := u.supersede(ctx, a, r); err != nil {
		return nil, err
	}
	u.notifier.Notify(u.notice(r))
	return u.pending(r), nil
}

func newRequest(a actor.Actor, c approval.Change) (entities.Request, error) {
	payload, err := json.Marshal(c.Proposal)
	if err != nil {
		return entities.Request{}, err
	}
	now := clock.Now().UTC()
	return entities.Request{
		ID: uuid.NewUUID(), Kind: c.Kind, Subject: c.Subject, ServiceID: c.ServiceID, ServiceSlug: c.ServiceSlug,
		TeamID: c.TeamID, Environment: c.Environment, Summary: entities.Clip(c.Summary, entities.SummaryLimit),
		Payload: payload, RequestedBy: a.UserID, RequesterLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name,
		Status: entities.StatusPending, CreatedAt: now, ExpiresAt: now.Add(entities.Lifetime),
	}, nil
}

func (u SubmitUsecase) supersede(ctx context.Context, a actor.Actor, newer entities.Request) error {
	older, err := u.store.PendingFor(ctx, newer.ServiceID, newer.Environment, newer.Subject)
	if err != nil {
		return err
	}
	for _, r := range older {
		if err := u.cancel(ctx, a, r, newer.ID); err != nil {
			return err
		}
	}
	return nil
}

func (u SubmitUsecase) cancel(ctx context.Context, a actor.Actor, older entities.Request, newerID string) error {
	if older.ID == newerID {
		return nil
	}
	closed := older.Close(entities.StatusCancelled, a, "superseded by "+newerID, clock.Now().UTC())
	err := u.store.Transition(ctx, closed, entities.StatusPending)
	if errors.Is(err, apperror.Conflict("")) {
		return nil
	}
	if err != nil {
		return err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, "request.superseded", closed))
	return nil
}

func (u SubmitUsecase) notice(r entities.Request) string {
	return fmt.Sprintf("%s via %s asks to %s on %s in %s. Approve or reject within 24 hours: %s",
		r.RequesterLogin, r.TokenName, r.Summary, r.ServiceSlug, r.Environment, u.baseURL+r.ApprovePath())
}

func (u SubmitUsecase) pending(r entities.Request) *approval.Pending {
	return &approval.Pending{
		Kind: "change_request", ID: r.ID, Status: string(r.Status), Summary: r.Summary,
		ApproveURL: u.baseURL + r.ApprovePath(), Poll: r.PollPath(), ExpiresAt: r.ExpiresAt,
	}
}
```

Create `app/approvals/usecases/requests/requests.go`:

```go
package requests

import (
	"context"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type Store interface {
	Get(ctx context.Context, id string) (entities.Request, error)
	List(ctx context.Context, f entities.Filter) ([]entities.Request, error)
	Overdue(ctx context.Context, now time.Time) ([]entities.Request, error)
	Transition(ctx context.Context, r entities.Request, from entities.Status) error
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type TokenActors interface {
	ActorOf(ctx context.Context, tokenID string) (actor.Actor, error)
}

type Executor interface {
	Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error)
	Preview(ctx context.Context, payload []byte) (approval.Preview, error)
}

type Executors map[string]Executor

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

const listLimit = 100

var (
	errNotVisible = apperror.NotFound("change request not found")
	errNotPending = apperror.Conflict("this request was already decided")
	errExpired    = apperror.Conflict("this request expired, ask for a new one")
	errNotYours   = apperror.Forbidden("only the person who asked can cancel a request, approvers reject it")
	errNoExecutor = apperror.Invalid("the platform does not know how to run this kind of request")
)

type RequestsUsecase struct {
	store     Store
	authz     Authorizer
	tokens    TokenActors
	executors Executors
	publisher Publisher
}

func NewRequestsUsecase(store Store, authz Authorizer, tokens TokenActors, executors Executors, publisher Publisher) RequestsUsecase {
	return RequestsUsecase{store: store, authz: authz, tokens: tokens, executors: executors, publisher: publisher}
}

func (u RequestsUsecase) Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.store.Get(ctx, id)
	if err != nil {
		return entities.Request{}, err
	}
	if !a.Sees(r.TeamID) {
		return entities.Request{}, errNotVisible
	}
	return r, nil
}

func (u RequestsUsecase) List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error) {
	return u.store.List(ctx, entities.Filter{Status: status, TeamIDs: a.Token.TeamIDs, Limit: listLimit})
}

func (u RequestsUsecase) Awaiting(ctx context.Context, a actor.Actor) ([]entities.Request, error) {
	pending, err := u.List(ctx, a, entities.StatusPending)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(pending, func(r entities.Request) bool { return !u.MayDecide(ctx, a, r) }), nil
}

func (u RequestsUsecase) MayDecide(ctx context.Context, a actor.Actor, r entities.Request) bool {
	return u.authz.Can(ctx, a, authz.ApproveRequest, authz.Resource{TeamID: r.TeamID}) == nil
}

func (u RequestsUsecase) Preview(ctx context.Context, r entities.Request) (approval.Preview, error) {
	executor, known := u.executors[r.Kind]
	if !known {
		return approval.Preview{}, errNoExecutor
	}
	return executor.Preview(ctx, r.Payload)
}
```

Create `app/approvals/usecases/requests/decide.go`:

```go
package requests

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

func (u RequestsUsecase) Approve(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.decidable(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	approved := r.Approve(a, clock.Now().UTC())
	if err := u.store.Transition(ctx, approved, entities.StatusPending); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, "request.approved", approved))
	return u.execute(ctx, a, approved)
}

func (u RequestsUsecase) Reject(ctx context.Context, a actor.Actor, id, reason string) (entities.Request, error) {
	r, err := u.decidable(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	return u.close(ctx, a, r, entities.StatusRejected, strings.TrimSpace(reason), "request.rejected")
}

func (u RequestsUsecase) Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.Get(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	if r.RequestedBy != a.UserID {
		return entities.Request{}, errNotYours
	}
	if !r.Pending() {
		return entities.Request{}, errNotPending
	}
	return u.close(ctx, a, r, entities.StatusCancelled, "cancelled by "+a.Login, "request.cancelled")
}

func (u RequestsUsecase) decidable(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.Get(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	if err := u.authz.Can(ctx, a, authz.ApproveRequest, authz.Resource{TeamID: r.TeamID}); err != nil {
		return entities.Request{}, err
	}
	if !r.Pending() {
		return entities.Request{}, errNotPending
	}
	if r.ExpiredAt(clock.Now().UTC()) {
		return entities.Request{}, errExpired
	}
	return r, nil
}

func (u RequestsUsecase) close(ctx context.Context, a actor.Actor, r entities.Request, status entities.Status, reason, action string) (entities.Request, error) {
	closed := r.Close(status, a, reason, clock.Now().UTC())
	if err := u.store.Transition(ctx, closed, entities.StatusPending); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, action, closed))
	return closed, nil
}

func (u RequestsUsecase) execute(ctx context.Context, approver actor.Actor, r entities.Request) (entities.Request, error) {
	result, err := u.run(ctx, approver, r)
	done, action := finished(r, result, err)
	if err := u.store.Transition(ctx, done, entities.StatusApproved); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(approver, action, done))
	return done, nil
}

func (u RequestsUsecase) run(ctx context.Context, approver actor.Actor, r entities.Request) (string, error) {
	executor, known := u.executors[r.Kind]
	if !known {
		return "", errNoExecutor
	}
	owner, err := u.tokens.ActorOf(ctx, r.TokenID)
	if err != nil {
		return "", err
	}
	owner.Approval = actor.Approval{RequestID: r.ID, ApproverID: approver.UserID, ApproverLogin: approver.Login}
	return executor.Execute(ctx, owner, r.Payload)
}

func finished(r entities.Request, result string, err error) (entities.Request, string) {
	now := clock.Now().UTC()
	if err != nil {
		return r.Fail(string(apperror.CodeOf(err)), messageOf(err), now), "request.failed"
	}
	return r.Succeed(result, now), "request.executed"
}

func messageOf(err error) string {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) && appErr.Code != apperror.CodeInternal {
		return appErr.Message
	}
	log.Printf("change request failed: %v", err)
	return "internal error, see the control plane logs"
}
```

Create `app/approvals/usecases/requests/sweep.go`:

```go
package requests

import (
	"context"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

var platform = actor.Actor{Login: "platform"}

const interrupted = "the platform stopped while running this request, check the target before asking again"

func (u RequestsUsecase) Sweep(ctx context.Context) (int, error) {
	overdue, err := u.store.Overdue(ctx, clock.Now().UTC())
	if err != nil {
		return 0, err
	}
	swept := 0
	for _, r := range overdue {
		if u.sweepOne(ctx, r) == nil {
			swept++
		}
	}
	return swept, nil
}

func (u RequestsUsecase) sweepOne(ctx context.Context, r entities.Request) error {
	if r.Pending() {
		_, err := u.close(ctx, platform, r, entities.StatusExpired, "nobody decided within 24 hours", "request.expired")
		return err
	}
	failed := r.Fail(string(apperror.CodeInternal), interrupted, clock.Now().UTC())
	if err := u.store.Transition(ctx, failed, entities.StatusApproved); err != nil {
		return err
	}
	u.publisher.Publish(ctx, entities.EventOf(platform, "request.failed", failed))
	return nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./app/... ./pkg/... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg app/authz app/envconfig app/deploy/usecases app/queue/usecases app/identity/usecases app/approvals/entities/event.go app/approvals/usecases
git commit -m "Turn agent changes to protected environments into requests a human approves"
```

---

### Task 7: Handlers — 202, Discord, executors, requests API, wiring, integration tests

**Files:**
- Modify: `internal/httperror/httperror_test.go`
- Modify: `pkg/config/config.go`
- Modify: `internal/httperror/httperror.go`
- Create: `app/approvals/repositories/discord/discord_test.go`
- Create: `app/approvals/repositories/discord/discord.go`
- Create: `app/approvals/repositories/envconfig/envconfig.go`
- Create: `app/approvals/repositories/deploy/deploy.go`
- Create: `app/approvals/repositories/queue/queue.go`
- Create: `app/approvals/handlers/requests/dto.go`
- Create: `app/approvals/handlers/requests/requests.go`
- Create: `cmd/controlplane/modules/approvals.go`
- Modify: `cmd/controlplane/modules/authz.go`
- Modify: `cmd/controlplane/options/options.go`
- Modify: `internal/integration/suite.go`
- Modify: `internal/integration/tests/token_access_test.go`
- Create: `internal/integration/tests/approvals_test.go`

`httperror.Abort` answers a wrapped `*approval.Pending` with `202`, so no domain handler changes. Executors decode a payload into the owning domain's proposal. The approvals module provides the `Requests` ports to the three domains, the executor map and the sweeper. The suite closes its database pool when a test ends.

**Interfaces:**
- Consumes: everything from Tasks 1–6.
- Produces:

```go
package config
type Approvals struct { WebhookURL string }

package discord
func NewNotifier(cfg config.Config) Notifier
func (n Notifier) Notify(text string)

package envconfig
func NewExecutor(configs Configs) Executor

package deploy
func NewExecutor(deployments Deployments) Executor

package queue
func NewExecutor(queues Queues) Executor

package requests
func NewRequestsHandler(cfg config.Config, requests Requests) RequestsHandler
func RegisterRequestsRoutes(api httpserver.API, h RequestsHandler)

package modules
func Approvals() fx.Option
```

Routes: `GET /api/v1/requests`, `GET /api/v1/requests/:id`, `POST /api/v1/requests/:id/cancel`. The integration tests use the web routes of Task 8 (`/requests/:id/approve`, `/requests/:id/reject`, `/requests/badge`, `/requests`, `/requests/:id`), so run them at the end of Task 8.


- [ ] **Step 1: Write the failing tests**

Modify `internal/httperror/httperror_test.go` (apply this diff; context lines are the current code):

```diff
--- a/internal/httperror/httperror_test.go
+++ b/internal/httperror/httperror_test.go
@@ -2,11 +2,15 @@ package httperror_test
 
 import (
 	"errors"
+	"fmt"
 	"net/http"
+	"net/http/httptest"
 	"testing"
 
 	"github.com/klzn-labs/kalzone/internal/httperror"
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
+	"github.com/gin-gonic/gin"
 	"github.com/stretchr/testify/require"
 )
 
@@ -16,6 +20,7 @@ func Test_Resolve(t *testing.T) {
 		apperror.NotFound("x"):                             http.StatusNotFound,
 		apperror.Invalid("x"):                              http.StatusBadRequest,
 		apperror.Conflict("x"):                             http.StatusConflict,
+		apperror.Stale("x"):                                http.StatusConflict,
 		apperror.New(apperror.CodeUnauthenticated, "x"):    http.StatusUnauthorized,
 		errors.Join(apperror.Invalid("x"), errors.New("")): http.StatusBadRequest,
 		errors.New("boom"):                                 http.StatusInternalServerError,
@@ -32,3 +37,16 @@ func Test_Resolve_HidesInternalMessages(t *testing.T) {
 
 	require.Equal(t, "internal error", body.Message)
 }
+
+func Test_Abort_AnswersAWaitingChangeWith202(t *testing.T) {
+	gin.SetMode(gin.TestMode)
+	w := httptest.NewRecorder()
+	c, _ := gin.CreateTestContext(w)
+	pending := &approval.Pending{Kind: "change_request", ID: "c1", Status: "pending", Poll: "/api/v1/requests/c1"}
+
+	httperror.Abort(c, fmt.Errorf("deploy: %w", pending))
+
+	require.Equal(t, http.StatusAccepted, w.Code)
+	require.Contains(t, w.Body.String(), `"kind":"change_request"`)
+	require.Contains(t, w.Body.String(), `"poll":"/api/v1/requests/c1"`)
+}
```

Create `app/approvals/repositories/discord/discord_test.go`:

```go
package discord_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/repositories/discord"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

type received struct {
	Content         string `json:"content"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

func webhook(t *testing.T, status int, release chan struct{}) (*httptest.Server, chan received) {
	t.Helper()
	got := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m received
		_ = json.NewDecoder(r.Body).Decode(&m)
		<-release
		got <- m
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func Test_Notify_PostsWithoutMentionsAndWithoutWaiting(t *testing.T) {
	release := make(chan struct{})
	server, got := webhook(t, http.StatusNoContent, release)
	n := discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: server.URL}})

	started := time.Now()
	n.Notify("bob via claude-local asks to deploy release #2 to prod")
	returnedIn := time.Since(started)
	close(release)

	m := <-got
	require.Less(t, returnedIn, 100*time.Millisecond)
	require.Equal(t, "bob via claude-local asks to deploy release #2 to prod", m.Content)
	require.Empty(t, m.AllowedMentions.Parse)
}

func Test_Notify_IsQuietWithoutAURLAndSurvivesAFailingWebhook(t *testing.T) {
	release := make(chan struct{})
	close(release)
	server, got := webhook(t, http.StatusInternalServerError, release)

	discord.NewNotifier(config.Config{}).Notify("nobody listens")
	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: server.URL}}).Notify("fails")
	discord.NewNotifier(config.Config{Approvals: config.Approvals{WebhookURL: "http://127.0.0.1:1/hook"}}).Notify("unreachable")

	require.Equal(t, "fails", (<-got).Content)
}
```

Modify `internal/integration/tests/token_access_test.go` (apply this diff; context lines are the current code):

```diff
--- a/internal/integration/tests/token_access_test.go
+++ b/internal/integration/tests/token_access_test.go
@@ -56,8 +56,8 @@ func Test_ProdNeedsProtectedAndApprovalUnlessBypassed(t *testing.T) {
 	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &all)
 
 	require.Contains(t, noScope.Body, `"required_scope":"protected"`)
-	require.Equal(t, http.StatusForbidden, noBypass.Status)
-	require.Contains(t, noBypass.Body, "err_approval_required")
+	require.Equal(t, http.StatusAccepted, noBypass.Status, noBypass.Body)
+	require.Contains(t, noBypass.Body, `"kind":"change_request"`)
 	require.Equal(t, http.StatusAccepted, bypassed.Status, bypassed.Body)
 	require.True(t, hasAction(all, "token.bypass_used"))
 }
```

Create `internal/integration/tests/approvals_test.go`:

```go
package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

type pendingChange struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	ApproveURL string `json:"approve_url"`
	Poll       string `json:"poll"`
}

type requestView struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Result string `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func prodAgent(t *testing.T, s *integration.Suite, p people) *integration.Client {
	t.Helper()
	return agentFor(t, s, p.mia, tokenBody("mia-agent", "read", "services", "config", "secrets", "deploy", "protected"))
}

func ask(t *testing.T, agent *integration.Client, method, path string, body any) pendingChange {
	t.Helper()
	var pending pendingChange
	agent.MustJSON(t, method, path, body, http.StatusAccepted, &pending)
	return pending
}

func poll(t *testing.T, c *integration.Client, pending pendingChange) requestView {
	t.Helper()
	var r requestView
	c.MustJSON(t, http.MethodGet, pending.Poll, nil, http.StatusOK, &r)
	return r
}

func decide(t *testing.T, c *integration.Client, pending pendingChange, verb string, values url.Values) integration.Response {
	t.Helper()
	values.Set("_csrf", c.CSRFToken)
	return c.Form(t, "/requests/"+pending.ID+"/"+verb, values)
}

func Test_AnAgentsProdDeployWaitsForAHuman(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release_id": releaseID})
	before := poll(t, agent, pending)
	approved := decide(t, p.mia, pending, "approve", url.Values{})
	after := poll(t, agent, pending)

	require.Equal(t, "change_request", pending.Kind)
	require.Equal(t, "http://platform.test/requests/"+pending.ID, pending.ApproveURL)
	require.Equal(t, "pending", before.Status)
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, "executed", after.Status)
	require.Contains(t, after.Result, "deployment #1")
}

func Test_OnlyAMaintainerOrAdminApprovesAndOnlyInTheBrowser(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	pending := askToSet(t, agent, "A", "1")

	byMember := decide(t, p.bob, pending, "approve", url.Values{})
	byToken := agent.Do(t, http.MethodPost, "/requests/"+pending.ID+"/approve", nil)
	byAdmin := decide(t, p.root, pending, "approve", url.Values{})

	require.Equal(t, http.StatusUnprocessableEntity, byMember.Status, byMember.Body)
	require.Equal(t, http.StatusUnauthorized, byToken.Status, byToken.Body)
	require.Equal(t, http.StatusNoContent, byAdmin.Status, byAdmin.Body)
	require.Equal(t, "executed", poll(t, agent, pending).Status)
}

func Test_ASecretRequestNeverShowsItsValue(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPut, "/api/v1/config/places-api/prod/secrets/DB_PASSWORD", map[string]string{"value": "hunter2-prod"})
	page := p.mia.Do(t, http.MethodGet, "/requests/"+pending.ID, nil)
	api := agent.Do(t, http.MethodGet, pending.Poll, nil)
	decide(t, p.mia, pending, "approve", url.Values{})
	secrets := p.mia.Do(t, http.MethodGet, "/api/v1/config/places-api/prod/secrets", nil)

	require.Contains(t, page.Body, "secret.DB_PASSWORD")
	require.Contains(t, page.Body, "new value")
	require.NotContains(t, page.Body, "hunter2-prod")
	require.NotContains(t, api.Body, "hunter2-prod")
	require.Contains(t, secrets.Body, "DB_PASSWORD")
}

func setProd(t *testing.T, c *integration.Client, key, value string) {
	t.Helper()
	c.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/prod/revisions", map[string]any{"set_env": map[string]string{key: value}}, http.StatusCreated, nil)
}

func askToSet(t *testing.T, agent *integration.Client, key, value string) pendingChange {
	t.Helper()
	return ask(t, agent, http.MethodPost, "/api/v1/config/places-api/prod/revisions", map[string]any{"set_env": map[string]string{key: value}})
}

func prodEnv(t *testing.T, c *integration.Client) map[string]string {
	t.Helper()
	var current struct {
		Env map[string]string `json:"env"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/config/places-api/prod", nil, http.StatusOK, &current)
	return current.Env
}

func Test_ApprovalReappliesOnTheCurrentConfigAndFailsOnARealConflict(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)

	clean := askToSet(t, agent, "A", "1")
	setProd(t, p.mia, "B", "2")
	decide(t, p.mia, clean, "approve", url.Values{})
	clash := askToSet(t, agent, "A", "3")
	setProd(t, p.mia, "A", "4")
	warning := p.mia.Do(t, http.MethodGet, "/requests/"+clash.ID, nil)
	decide(t, p.mia, clash, "approve", url.Values{})

	failed := poll(t, agent, clash)
	require.Equal(t, "executed", poll(t, agent, clean).Status)
	require.Contains(t, warning.Body, "approving will fail")
	require.Equal(t, "failed", failed.Status)
	require.Equal(t, "err_stale", failed.Error.Code)
	require.Equal(t, map[string]string{"A": "4", "B": "2"}, prodEnv(t, p.mia))
}

func Test_ANewerRequestSupersedesTheOlderOne(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	agent := prodAgent(t, s, p)
	deploy := map[string]string{"service": "places-api", "environment": "prod", "release_id": releaseID}

	first := ask(t, agent, http.MethodPost, "/api/v1/deployments", deploy)
	second := ask(t, agent, http.MethodPost, "/api/v1/deployments", deploy)

	older := poll(t, agent, first)
	require.Equal(t, "cancelled", older.Status)
	require.Equal(t, "superseded by "+second.ID, older.Reason)
	require.Equal(t, "pending", poll(t, agent, second).Status)
}

func Test_RejectionsAndCancellationsReachTheAgent(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	stranger := agentFor(t, s, p.bob, tokenBody("bob-reader", "read"))

	rejected := askToSet(t, agent, "A", "1")
	decide(t, p.mia, rejected, "reject", url.Values{"reason": {"not during the sale"}})
	cancelled := ask(t, agent, http.MethodPut, "/api/v1/config/places-api/prod/secrets/TOKEN", map[string]string{"value": "x"})
	notTheirs := stranger.Do(t, http.MethodPost, "/api/v1/requests/"+cancelled.ID+"/cancel", nil)
	agent.MustJSON(t, http.MethodPost, "/api/v1/requests/"+cancelled.ID+"/cancel", nil, http.StatusOK, nil)

	require.Equal(t, "rejected", poll(t, agent, rejected).Status)
	require.Equal(t, "not during the sale", poll(t, agent, rejected).Reason)
	require.Equal(t, http.StatusForbidden, notTheirs.Status, notTheirs.Body)
	require.Equal(t, "cancelled", poll(t, agent, cancelled).Status)
}

func Test_TheNavCountsWhatTheViewerMayApprove(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	askToSet(t, agent, "A", "1")

	mia := p.mia.Do(t, http.MethodGet, "/requests/badge", nil)
	bob := p.bob.Do(t, http.MethodGet, "/requests/badge", nil)
	list := p.bob.Do(t, http.MethodGet, "/requests", nil)

	require.Contains(t, mia.Body, ">1<")
	require.Empty(t, bob.Body)
	require.Contains(t, list.Body, "change env.A")
}

func Test_QueueEditsOnProdWaitToo(t *testing.T) {
	s, p := setupQueue(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/queues", map[string]any{
		"name": "place-import", "environment": "prod", "consumer": "places-worker", "path": "/hooks/place-import",
	})
	decide(t, p.mia, pending, "approve", url.Values{})
	var q queueDTO
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/queues/prod/place-import", nil, http.StatusOK, &q)

	require.Equal(t, "queue version 1", poll(t, agent, pending).Result)
	require.Equal(t, "places-worker", q.Consumer)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./internal/httperror/ ./app/approvals/repositories/discord/`
Expected: FAIL: `Test_Resolve` gets 500 for `err_stale`, `Test_Abort_AnswersAWaitingChangeWith202` gets 500; the `discord` package does not exist yet

- [ ] **Step 3: Implement**

Modify `pkg/config/config.go` (apply this diff; context lines are the current code):

```diff
--- a/pkg/config/config.go
+++ b/pkg/config/config.go
@@ -12,20 +12,25 @@ import (
 )
 
 type Config struct {
-	HTTP           HTTP     `mapstructure:"http"`
-	BaseURL        string   `mapstructure:"base_url" default:"http://localhost:8080" validate:"required,url"`
-	DB             DB       `mapstructure:"db"`
-	GitHub         GitHub   `mapstructure:"github"`
-	Admins         []string `mapstructure:"admins"`
-	Secrets        Secrets  `mapstructure:"secrets"`
-	AllowedDomains []string `mapstructure:"allowed_domains" default:"[\"localhost\"]" validate:"min=1"`
-	Session        Session  `mapstructure:"session"`
-	OIDC           OIDC     `mapstructure:"oidc"`
-	Build          Build    `mapstructure:"build"`
-	Deploy         Deploy   `mapstructure:"deploy"`
-	Agent          Agent    `mapstructure:"agent"`
-	Grafana        Grafana  `mapstructure:"grafana"`
-	Queues         Queues   `mapstructure:"queues"`
+	HTTP           HTTP      `mapstructure:"http"`
+	BaseURL        string    `mapstructure:"base_url" default:"http://localhost:8080" validate:"required,url"`
+	DB             DB        `mapstructure:"db"`
+	GitHub         GitHub    `mapstructure:"github"`
+	Admins         []string  `mapstructure:"admins"`
+	Secrets        Secrets   `mapstructure:"secrets"`
+	AllowedDomains []string  `mapstructure:"allowed_domains" default:"[\"localhost\"]" validate:"min=1"`
+	Session        Session   `mapstructure:"session"`
+	OIDC           OIDC      `mapstructure:"oidc"`
+	Build          Build     `mapstructure:"build"`
+	Deploy         Deploy    `mapstructure:"deploy"`
+	Agent          Agent     `mapstructure:"agent"`
+	Grafana        Grafana   `mapstructure:"grafana"`
+	Queues         Queues    `mapstructure:"queues"`
+	Approvals      Approvals `mapstructure:"approvals"`
+}
+
+type Approvals struct {
+	WebhookURL string `mapstructure:"webhook_url" validate:"omitempty,url"`
 }
 
 type Queues struct {
@@ -128,7 +133,7 @@ var envKeys = []string{
 	"session.ttl", "session.renew_within",
 	"github.app_id", "github.app_private_key", "github.webhook_secret",
 	"oidc.issuer", "oidc.audience", "build.timeout", "deploy.timeout", "agent.token", "grafana.url", "grafana.token",
-	"queues.gateway_url", "queues.token",
+	"queues.gateway_url", "queues.token", "approvals.webhook_url",
 }
 
 func bindEnvVars() {
```

Modify `internal/httperror/httperror.go` (apply this diff; context lines are the current code):

```diff
--- a/internal/httperror/httperror.go
+++ b/internal/httperror/httperror.go
@@ -6,6 +6,7 @@ import (
 	"net/http"
 
 	"github.com/klzn-labs/kalzone/pkg/apperror"
+	"github.com/klzn-labs/kalzone/pkg/approval"
 	"github.com/gin-gonic/gin"
 )
 
@@ -23,6 +24,7 @@ var statuses = map[apperror.Code]int{
 	apperror.CodeMissingScope:     http.StatusForbidden,
 	apperror.CodeTeamNotGranted:   http.StatusForbidden,
 	apperror.CodeApprovalRequired: http.StatusForbidden,
+	apperror.CodeStale:            http.StatusConflict,
 	apperror.CodeTokenInvalid:     http.StatusUnauthorized,
 	apperror.CodeTokenExpired:     http.StatusUnauthorized,
 	apperror.CodeTokenRevoked:     http.StatusUnauthorized,
@@ -49,6 +51,11 @@ type Body struct {
 }
 
 func Abort(c *gin.Context, err error) {
+	var pending *approval.Pending
+	if errors.As(err, &pending) {
+		c.AbortWithStatusJSON(http.StatusAccepted, pending)
+		return
+	}
 	status, body := Resolve(err)
 	c.AbortWithStatusJSON(status, Body{Error: body})
 }
```

Create `app/approvals/repositories/discord/discord.go`:

```go
package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/klzn-labs/kalzone/pkg/config"
)

const timeout = 10 * time.Second

type message struct {
	Content         string   `json:"content"`
	AllowedMentions mentions `json:"allowed_mentions"`
}

type mentions struct {
	Parse []string `json:"parse"`
}

type Notifier struct {
	url    string
	client *http.Client
}

func NewNotifier(cfg config.Config) Notifier {
	return Notifier{url: cfg.Approvals.WebhookURL, client: &http.Client{Timeout: timeout}}
}

func (n Notifier) Notify(text string) {
	if n.url == "" {
		return
	}
	go n.send(text)
}

func (n Notifier) send(text string) {
	if err := n.post(text); err != nil {
		log.Printf("approval notification not sent: %v", withoutURL(err))
	}
}

func (n Notifier) post(text string) error {
	body, err := json.Marshal(message{Content: text, AllowedMentions: mentions{Parse: []string{}}})
	if err != nil {
		return err
	}
	resp, err := n.client.Post(n.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("the webhook answered %s", resp.Status)
	}
	return nil
}

func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
```

Create `app/approvals/repositories/envconfig/envconfig.go`:

```go
package envconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Configs interface {
	ApplyApproved(ctx context.Context, a actor.Actor, p configs.Proposal) (entities.Revision, error)
	Preview(ctx context.Context, p configs.Proposal) (approval.Preview, error)
}

type Executor struct {
	configs Configs
}

func NewExecutor(configs Configs) Executor {
	return Executor{configs: configs}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var p configs.Proposal
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	rev, err := e.configs.ApplyApproved(ctx, a, p)
	return fmt.Sprintf("config revision #%d", rev.Number), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var p configs.Proposal
	if err := json.Unmarshal(payload, &p); err != nil {
		return approval.Preview{}, err
	}
	return e.configs.Preview(ctx, p)
}
```

Create `app/approvals/repositories/deploy/deploy.go`:

```go
package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Deployments interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o deployments.Proposal) (entities.Deployment, error)
	Preview(ctx context.Context, o deployments.Proposal) (approval.Preview, error)
}

type Executor struct {
	deployments Deployments
}

func NewExecutor(deployments Deployments) Executor {
	return Executor{deployments: deployments}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o deployments.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	d, err := e.deployments.ApplyApproved(ctx, a, o)
	return fmt.Sprintf("deployment #%d (%s)", d.Number, d.ID), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o deployments.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.deployments.Preview(ctx, o)
}
```

Create `app/approvals/repositories/queue/queue.go`:

```go
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Queues interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o queues.Proposal) (entities.Queue, error)
	Preview(ctx context.Context, o queues.Proposal) (approval.Preview, error)
}

type Executor struct {
	queues Queues
}

func NewExecutor(queues Queues) Executor {
	return Executor{queues: queues}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o queues.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	q, err := e.queues.ApplyApproved(ctx, a, o)
	if o.Kind == approval.KindQueueDelete {
		return "queue deleted", err
	}
	return fmt.Sprintf("queue version %d", q.Version), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o queues.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.queues.Preview(ctx, o)
}
```

Create `app/approvals/handlers/requests/dto.go`:

```go
package requests

import (
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
)

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type requestDTO struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Service     string     `json:"service"`
	Environment string     `json:"environment"`
	Summary     string     `json:"summary"`
	RequestedBy string     `json:"requested_by"`
	Token       string     `json:"token"`
	ApproveURL  string     `json:"approve_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	DecidedBy   string     `json:"decided_by,omitempty"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Result      string     `json:"result,omitempty"`
	Error       *errorDTO  `json:"error,omitempty"`
}

func toDTO(r entities.Request, baseURL string) requestDTO {
	dto := requestDTO{
		ID: r.ID, Kind: r.Kind, Status: string(r.Status), Service: r.ServiceSlug, Environment: r.Environment,
		Summary: r.Summary, RequestedBy: r.RequesterLogin, Token: r.TokenName, ApproveURL: baseURL + r.ApprovePath(),
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, DecidedBy: r.DeciderLogin, DecidedAt: r.DecidedAt,
		Reason: r.Reason, Result: r.Result,
	}
	if r.ErrorCode != "" {
		dto.Error = &errorDTO{Code: r.ErrorCode, Message: r.ErrorMessage}
	}
	return dto
}
```

Create `app/approvals/handlers/requests/requests.go`:

```go
package requests

import (
	"context"
	"net/http"
	"strings"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/gin-gonic/gin"
)

type Requests interface {
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
}

type RequestsHandler struct {
	requests Requests
	baseURL  string
}

func NewRequestsHandler(cfg config.Config, requests Requests) RequestsHandler {
	return RequestsHandler{requests: requests, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
}

func (h RequestsHandler) List(c *gin.Context) {
	list, err := h.requests.List(c.Request.Context(), requestctx.ActorFrom(c), entities.Status(c.Query("status")))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]requestDTO, 0, len(list))
	for _, r := range list {
		result = append(result, toDTO(r, h.baseURL))
	}
	c.JSON(http.StatusOK, gin.H{"requests": result})
}

func (h RequestsHandler) Get(c *gin.Context) {
	r, err := h.requests.Get(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.respond(c, r, err)
}

func (h RequestsHandler) Cancel(c *gin.Context) {
	r, err := h.requests.Cancel(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.respond(c, r, err)
}

func (h RequestsHandler) respond(c *gin.Context, r entities.Request, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDTO(r, h.baseURL))
}

func RegisterRequestsRoutes(api httpserver.API, h RequestsHandler) {
	api.GET("/requests", h.List)
	api.GET("/requests/:id", h.Get)
	api.POST("/requests/:id/cancel", h.Cancel)
}
```

Create `cmd/controlplane/modules/approvals.go`:

```go
package modules

import (
	requestsHandler "github.com/klzn-labs/kalzone/app/approvals/handlers/requests"
	approvalsDeploy "github.com/klzn-labs/kalzone/app/approvals/repositories/deploy"
	"github.com/klzn-labs/kalzone/app/approvals/repositories/discord"
	approvalsConfig "github.com/klzn-labs/kalzone/app/approvals/repositories/envconfig"
	approvalsMysql "github.com/klzn-labs/kalzone/app/approvals/repositories/mysql"
	approvalsQueue "github.com/klzn-labs/kalzone/app/approvals/repositories/queue"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/submit"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"go.uber.org/fx"
)

func Approvals() fx.Option {
	return fx.Options(
		fx.Provide(
			approvalsMysql.NewRequestRepository, discord.NewNotifier, submit.NewSubmitUsecase, requests.NewRequestsUsecase,
			approvalsConfig.NewExecutor, approvalsDeploy.NewExecutor, approvalsQueue.NewExecutor, executors,
			requestsHandler.NewRequestsHandler,
		),
		approvalsInjections(),
		submitInjections(),
		fx.Invoke(requestsHandler.RegisterRequestsRoutes, startApprovalsSweeper),
	)
}

func approvalsInjections() fx.Option {
	return fx.Provide(
		func(r approvalsMysql.RequestRepository) submit.Store { return r },
		func(r approvalsMysql.RequestRepository) requests.Store { return r },
		func(n discord.Notifier) submit.Notifier { return n },
		func(b *eventbus.Bus) submit.Publisher { return b },
		func(b *eventbus.Bus) requests.Publisher { return b },
		func(p authz.Policy) requests.Authorizer { return p },
		func(u tokens.TokensUsecase) requests.TokenActors { return u },
		func(u configs.ConfigUsecase) approvalsConfig.Configs { return u },
		func(u deployments.DeploymentsUsecase) approvalsDeploy.Deployments { return u },
		func(u queues.QueuesUsecase) approvalsQueue.Queues { return u },
		func(u requests.RequestsUsecase) requestsHandler.Requests { return u },
	)
}

func submitInjections() fx.Option {
	return fx.Provide(
		func(u submit.SubmitUsecase) configs.Requests { return u },
		func(u submit.SubmitUsecase) deployments.Requests { return u },
		func(u submit.SubmitUsecase) queues.Requests { return u },
	)
}

func executors(c approvalsConfig.Executor, d approvalsDeploy.Executor, q approvalsQueue.Executor) requests.Executors {
	return requests.Executors{
		approval.KindConfigRevise: c, approval.KindSecretSet: c, approval.KindSecretUnset: c,
		approval.KindDeploy: d, approval.KindPromote: d, approval.KindRollback: d, approval.KindConfigApply: d,
		approval.KindQueueCreate: q, approval.KindQueueUpdate: q, approval.KindQueueDelete: q,
	}
}

func startApprovalsSweeper(lc fx.Lifecycle, u requests.RequestsUsecase) {
	sweeper.Start(lc, "approvals", u)
}
```

Modify `cmd/controlplane/modules/authz.go` (apply this diff; context lines are the current code):

```diff
--- a/cmd/controlplane/modules/authz.go
+++ b/cmd/controlplane/modules/authz.go
@@ -4,7 +4,6 @@ import (
 	"github.com/klzn-labs/kalzone/app/authz"
 	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
 	"github.com/klzn-labs/kalzone/app/team/usecases/source"
-	"github.com/klzn-labs/kalzone/internal/eventbus"
 	"go.uber.org/fx"
 )
 
@@ -13,6 +12,5 @@ func Authz() fx.Option {
 		authz.NewPolicy,
 		func(s source.LocalTeamSource) authz.MembershipReader { return s },
 		func(u environments.EnvironmentsUsecase) authz.ProtectionReader { return u },
-		func(b *eventbus.Bus) authz.Publisher { return b },
 	)
 }
```

Modify `cmd/controlplane/options/options.go` (apply this diff; context lines are the current code):

```diff
--- a/cmd/controlplane/options/options.go
+++ b/cmd/controlplane/options/options.go
@@ -23,6 +23,7 @@ func Options() fx.Option {
 		modules.Release(),
 		modules.Queue(),
 		modules.Deploy(),
+		modules.Approvals(),
 		modules.Observe(),
 		modules.Web(),
 	)
```

Modify `internal/integration/suite.go` (apply this diff; context lines are the current code):

```diff
--- a/internal/integration/suite.go
+++ b/internal/integration/suite.go
@@ -21,6 +21,7 @@ import (
 	"go.uber.org/fx"
 	"go.uber.org/fx/fxevent"
 	"go.uber.org/fx/fxtest"
+	"gorm.io/gorm"
 )
 
 const (
@@ -37,6 +38,7 @@ type Suite struct {
 	Registry *fakeregistry.Registry
 	Grafana  *fakegrafana.Grafana
 	Engine   *gin.Engine
+	db       *gorm.DB
 	nextID   int64
 }
 
@@ -50,7 +52,7 @@ func NewSuite(t *testing.T) *Suite {
 		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
 		fx.Decorate(func(config.Config) config.Config { return testConfig(t, s) }),
 		fx.Decorate(func(registry.Resolver) registry.Resolver { return registry.WithBaseURL(s.Registry.URL) }),
-		fx.Populate(&s.Engine),
+		fx.Populate(&s.Engine, &s.db),
 	)
 	app.RequireStart()
 	s.Server = httptest.NewServer(s.Engine)
@@ -65,6 +67,7 @@ func (s *Suite) close(app *fxtest.App) {
 	s.Registry.Close()
 	s.Grafana.Close()
 	app.RequireStop()
+	testdb.Close(s.db)
 }
 
 func testConfig(t *testing.T, s *Suite) config.Config {
```

- [ ] **Step 4: Run the tests**

Run: `go test -p 1 -count=1 ./internal/httperror/ ./app/approvals/... ./pkg/... && go vet ./...`
Expected: PASS (integration tests are run in Task 8)

---

### Task 8: UI — Requests page, request page, nav counter, audit lines

**Files:**
- Modify: `app/web/activity_test.go`
- Modify: `app/web/activity.go`
- Create: `app/web/handlers/requests/requests.go`
- Modify: `app/web/templates/partials/components.html`
- Create: `app/web/templates/pages/requests.html`
- Create: `app/web/templates/pages/request.html`
- Modify: `app/web/templates/layout.html`
- Modify: `app/web/static/app.css`
- Modify: `cmd/controlplane/modules/web.go`

The approve, reject and cancel routes are on the web group, so Bearer tokens are refused there by the existing guard. The nav counter loads from `/requests/badge` by htmx, so no page handler changes. Audit lines for request events link to the request.

**Interfaces:**
- Consumes: `requests.RequestsUsecase` (Task 6), `entities.Request.ApprovePath`, `approval.Preview`.
- Produces: `requestsPage.NewRequestsHandler(renderer *web.Renderer, requests Requests) RequestsHandler`, `RegisterRequestsRoutes(routes httpserver.Web, h RequestsHandler)`, template `request-status`.


- [ ] **Step 1: Show the mockup first**

Per the user's standing preference for visual changes, build a Design artifact with the Requests list and the request page (desktop and phone width, the platform's palette from `app/web/static/app.css`): the default option is exactly the templates below; add one or two alternatives (for example the decision buttons in the header instead of two cards). Share the link and wait for the pick, then adapt the templates of Step 4 to it.


- [ ] **Step 2: Write the failing tests**

Modify `app/web/activity_test.go` (apply this diff; context lines are the current code):

```diff
--- a/app/web/activity_test.go
+++ b/app/web/activity_test.go
@@ -36,3 +36,15 @@ func Test_ActivityFrom_KeepsTheTokenAndPhrasesBypass(t *testing.T) {
 	require.Equal(t, "t1", got[0].TokenID)
 	require.Equal(t, "deploy on prod", got[1].Detail)
 }
+
+func Test_ActivityFrom_LinksRequestEventsToTheRequest(t *testing.T) {
+	events := []audit.Event{
+		{ActorLogin: "mia", Action: "request.approved", TargetID: "c1", TargetName: "api", Details: map[string]any{"summary": "deploy release #2 to prod"}},
+	}
+
+	got := web.ActivityFrom(events)
+
+	require.Equal(t, "approved a request on", got[0].Phrase)
+	require.Equal(t, "/requests/c1", got[0].Link)
+	require.Equal(t, "deploy release #2 to prod", got[0].Detail)
+}
```

- [ ] **Step 3: Run them to see them fail**

Run: `go test -count=1 ./app/web/`
Expected: FAIL: `Test_ActivityFrom_LinksRequestEventsToTheRequest` gets phrase `request.approved`

- [ ] **Step 4: Implement**

Modify `app/web/activity.go` (apply this diff; context lines are the current code):

```diff
--- a/app/web/activity.go
+++ b/app/web/activity.go
@@ -49,6 +49,14 @@ var phrasings = map[string]phrasing{
 	"token.created":       {"created access token", tokensLink, nil},
 	"token.revoked":       {"revoked access token", tokensLink, nil},
 	"token.bypass_used":   {"skipped approval with", nil, bypassDetail},
+	"request.created":     {"asked for approval on", requestLink, requestDetail},
+	"request.approved":    {"approved a request on", requestLink, requestDetail},
+	"request.rejected":    {"rejected a request on", requestLink, requestDetail},
+	"request.cancelled":   {"cancelled a request on", requestLink, requestDetail},
+	"request.superseded":  {"replaced a request on", requestLink, requestDetail},
+	"request.expired":     {"expired a request on", requestLink, requestDetail},
+	"request.executed":    {"ran an approved change on", requestLink, requestDetail},
+	"request.failed":      {"could not run an approved change on", requestLink, requestDetail},
 }
 
 func ActivityFrom(events []audit.Event) []Activity {
@@ -77,6 +85,12 @@ func activityOf(e audit.Event) Activity {
 
 func tokensLink(audit.Event) string { return "/tokens" }
 
+func requestLink(e audit.Event) string { return "/requests/" + e.TargetID }
+
+func requestDetail(e audit.Event) string {
+	return fmt.Sprint(e.Details["summary"])
+}
+
 func bypassDetail(e audit.Event) string {
 	return fmt.Sprintf("%v on %s", e.Details["action"], e.Environment)
 }
```

Create `app/web/handlers/requests/requests.go`:

```go
package requests

import (
	"context"
	"fmt"
	"net/http"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/gin-gonic/gin"
)

type Requests interface {
	List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error)
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	Awaiting(ctx context.Context, a actor.Actor) ([]entities.Request, error)
	MayDecide(ctx context.Context, a actor.Actor, r entities.Request) bool
	Preview(ctx context.Context, r entities.Request) (approval.Preview, error)
	Approve(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	Reject(ctx context.Context, a actor.Actor, id, reason string) (entities.Request, error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
}

type listView struct {
	Pending []entities.Request
	Recent  []entities.Request
}

type detailView struct {
	Request      entities.Request
	Preview      approval.Preview
	PreviewError string
	CanDecide    bool
	CanCancel    bool
}

type RequestsHandler struct {
	renderer *web.Renderer
	requests Requests
}

func NewRequestsHandler(renderer *web.Renderer, requests Requests) RequestsHandler {
	return RequestsHandler{renderer: renderer, requests: requests}
}

func (h RequestsHandler) List(c *gin.Context) {
	all, err := h.requests.List(c.Request.Context(), requestctx.ActorFrom(c), "")
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	view := listView{}
	for _, r := range all {
		if r.Pending() {
			view.Pending = append(view.Pending, r)
			continue
		}
		view.Recent = append(view.Recent, r)
	}
	h.renderer.Render(c, http.StatusOK, "requests", web.Page{Title: "Requests", Nav: "requests", Data: view})
}

func (h RequestsHandler) Show(c *gin.Context) {
	ctx, a := c.Request.Context(), requestctx.ActorFrom(c)
	r, err := h.requests.Get(ctx, a, c.Param("id"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	view := detailView{Request: r, CanDecide: r.Pending() && h.requests.MayDecide(ctx, a, r), CanCancel: r.Pending() && r.RequestedBy == a.UserID}
	if r.Pending() {
		view.Preview, err = h.requests.Preview(ctx, r)
	}
	if err != nil {
		view.PreviewError = web.UserMessage(err)
	}
	h.renderer.Render(c, http.StatusOK, "request", web.Page{Title: r.Summary, Nav: "requests", Data: view})
}

func (h RequestsHandler) Badge(c *gin.Context) {
	waiting, err := h.requests.Awaiting(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil || len(waiting) == 0 {
		c.Data(http.StatusOK, "text/html; charset=utf-8", nil)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(`<span class="badge badge-warn">%d</span>`, len(waiting))))
}

func (h RequestsHandler) Approve(c *gin.Context) {
	r, err := h.requests.Approve(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	if r.Status == entities.StatusFailed {
		web.Done(c, r.ApprovePath(), "Approved, but it failed: "+r.ErrorMessage)
		return
	}
	web.Done(c, r.ApprovePath(), "Approved and done: "+r.Result)
}

func (h RequestsHandler) Reject(c *gin.Context) {
	r, err := h.requests.Reject(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"), c.PostForm("reason"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, r.ApprovePath(), "Rejected")
}

func (h RequestsHandler) Cancel(c *gin.Context) {
	r, err := h.requests.Cancel(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, r.ApprovePath(), "Cancelled")
}

func RegisterRequestsRoutes(routes httpserver.Web, h RequestsHandler) {
	routes.GET("/requests", h.List)
	routes.GET("/requests/badge", h.Badge)
	routes.GET("/requests/:id", h.Show)
	routes.POST("/requests/:id/approve", h.Approve)
	routes.POST("/requests/:id/reject", h.Reject)
	routes.POST("/requests/:id/cancel", h.Cancel)
}
```

Modify `app/web/templates/partials/components.html` (apply this diff; context lines are the current code):

```diff
--- a/app/web/templates/partials/components.html
+++ b/app/web/templates/partials/components.html
@@ -20,3 +20,5 @@
   </div>
   <time class="when" datetime="{{iso .At}}" title="{{date .At}}">{{ago .At}}</time>
 </li>{{end}}
+
+{{define "request-status"}}{{$s := print .}}<span class="badge {{if eq $s "pending"}}badge-warn{{else if eq $s "executed"}}badge-ok{{else if or (eq $s "failed") (eq $s "rejected")}}badge-danger{{else if eq $s "approved"}}badge-accent{{else}}badge-outline{{end}}"><span class="dot"></span>{{$s}}</span>{{end}}
```

Create `app/web/templates/pages/requests.html`:

```html
{{define "content"}}{{$d := .Data}}
<div class="page-header">
  <div>
    <div class="eyebrow">Workspace</div>
    <h1>Requests</h1>
    <p class="lead">Changes that agents asked to make on protected environments. A maintainer of the owning team or an admin approves or rejects each one in the browser. Unanswered requests expire after 24 hours.</p>
  </div>
</div>

<h2>Waiting</h2>
<div class="card" style="margin:12px 0 24px">
  {{if $d.Pending}}{{template "request-table" $d.Pending}}{{else}}{{template "empty" dict "Icon" "shield" "Title" "Nothing is waiting" "Text" "When an agent asks to change a protected environment, the request shows up here."}}{{end}}
</div>

<h2>Recent</h2>
<div class="card" style="margin-top:12px">
  {{if $d.Recent}}{{template "request-table" $d.Recent}}{{else}}<div class="card-body muted">No decided requests yet.</div>{{end}}
</div>
{{end}}

{{define "request-table"}}
<div class="table-scroll"><table class="table">
  <thead><tr><th>Change</th><th>Service</th><th>Env</th><th>Asked by</th><th>When</th><th>Status</th></tr></thead>
  <tbody>{{range .}}
    <tr>
      <td><a href="/requests/{{.ID}}"><b>{{.Summary}}</b></a></td>
      <td class="mono">{{.ServiceSlug}}</td>
      <td>{{template "env-badge" .Environment}}</td>
      <td>@{{.RequesterLogin}} <span class="muted">via {{.TokenName}}</span></td>
      <td class="muted nowrap" title="{{date .CreatedAt}}">{{ago .CreatedAt}}</td>
      <td>{{template "request-status" .Status}}</td>
    </tr>{{end}}
  </tbody>
</table></div>
{{end}}
```

Create `app/web/templates/pages/request.html`:

```html
{{define "content"}}{{$d := .Data}}{{$r := $d.Request}}{{$csrf := .Viewer.CSRFToken}}
<div class="page-header">
  <div>
    <div class="eyebrow"><a href="/requests">Requests</a></div>
    <h1>{{$r.Summary}}</h1>
    <p class="lead">@{{$r.RequesterLogin}} via <span class="mono">{{$r.TokenName}}</span> asked {{ago $r.CreatedAt}} to change <span class="mono">{{$r.ServiceSlug}}</span> in {{$r.Environment}}.</p>
  </div>
  <div class="actions">{{template "request-status" $r.Status}}</div>
</div>

<dl class="kv" style="margin-bottom:20px">
  <div><dt>Service</dt><dd>{{$r.ServiceSlug}}</dd></div>
  <div><dt>Environment</dt><dd>{{$r.Environment}}</dd></div>
  <div><dt>Kind</dt><dd>{{$r.Kind}}</dd></div>
  <div><dt>Asked</dt><dd>{{date $r.CreatedAt}}</dd></div>
  <div><dt>{{if $r.Pending}}Expires{{else}}Decided{{end}}</dt><dd>{{if $r.Pending}}{{date $r.ExpiresAt}}{{else if $r.DecidedAt}}{{date (deref $r.DecidedAt)}} by @{{$r.DeciderLogin}}{{end}}</dd></div>
  <div><dt>Request</dt><dd>{{$r.ID}}</dd></div>
</dl>

{{if $r.Pending}}
{{if $d.Preview.Moved}}<div class="alert alert-warn" style="margin-bottom:12px">{{icon "alert"}}<span>{{$d.Preview.Moved}}</span></div>{{end}}
{{if $d.Preview.Conflict}}<div class="alert alert-danger" style="margin-bottom:12px">{{icon "alert"}}<span>{{$d.Preview.Conflict}}</span></div>{{end}}
{{if $d.PreviewError}}<div class="alert alert-danger" style="margin-bottom:12px">{{icon "alert"}}<span>{{$d.PreviewError}}</span></div>{{end}}
<div class="card" style="margin-bottom:20px">
  <div class="card-header"><div><h2>What changes</h2><p>Compared with what runs now, not with what the agent saw.</p></div></div>
  <div class="table-scroll"><table class="table">
    <thead><tr><th>Key</th><th>Now</th><th>After approval</th></tr></thead>
    <tbody>{{range $d.Preview.Lines}}<tr><td class="key">{{.Key}}</td><td class="value">{{or .Before "—"}}</td><td class="value">{{or .After "—"}}</td></tr>{{else}}<tr><td colspan="3" class="muted">Nothing would change.</td></tr>{{end}}</tbody>
  </table></div>
</div>
{{if $d.CanDecide}}
<div class="grid grid-2">
  <form class="card" method="post" action="/requests/{{$r.ID}}/approve" hx-post="/requests/{{$r.ID}}/approve" hx-confirm="Approve and run it now?">{{template "csrf" $csrf}}
    <div class="card-body"><p>Runs the change now as @{{$r.RequesterLogin}} via {{$r.TokenName}}, with you as the approver.</p><div class="form-actions"><button class="btn btn-primary">{{icon "check"}}Approve and run</button></div></div>
  </form>
  <form class="card" method="post" action="/requests/{{$r.ID}}/reject" hx-post="/requests/{{$r.ID}}/reject">{{template "csrf" $csrf}}
    <div class="card-body"><div class="field"><label for="reason">Reason</label><input id="reason" name="reason" type="text" maxlength="500" placeholder="Optional, the agent reads it"></div><div class="form-actions"><button class="btn btn-danger">{{icon "x"}}Reject</button></div></div>
  </form>
</div>
{{end}}
{{if $d.CanCancel}}<form method="post" action="/requests/{{$r.ID}}/cancel" hx-post="/requests/{{$r.ID}}/cancel" style="margin-top:12px">{{template "csrf" $csrf}}<button class="btn btn-ghost">{{icon "trash"}}Cancel my request</button></form>{{end}}
{{else}}
<div class="card"><div class="card-body">
  {{if $r.Result}}<p>{{icon "check"}} Done: <b class="mono">{{$r.Result}}</b></p>{{end}}
  {{if $r.ErrorCode}}<div class="alert alert-danger">{{icon "alert"}}<span><span class="mono">{{$r.ErrorCode}}</span> · {{$r.ErrorMessage}}</span></div>{{end}}
  {{if $r.Reason}}<p class="muted">{{$r.Reason}}</p>{{end}}
</div></div>
{{end}}
{{end}}
```

Modify `app/web/templates/layout.html` (apply this diff; context lines are the current code):

```diff
--- a/app/web/templates/layout.html
+++ b/app/web/templates/layout.html
@@ -21,6 +21,7 @@
     <a class="nav-link {{if eq .Nav "teams"}}active{{end}}" href="/teams">{{icon "users"}}Teams</a>
     <a class="nav-link {{if eq .Nav "people"}}active{{end}}" href="/people">{{icon "user"}}People</a>
     <a class="nav-link {{if eq .Nav "queues"}}active{{end}}" href="/queues">{{icon "layers"}}Queues</a>
+    <a class="nav-link {{if eq .Nav "requests"}}active{{end}}" href="/requests">{{icon "shield"}}Requests<span class="nav-count" hx-get="/requests/badge" hx-trigger="load, every 30s"></span></a>
     <a class="nav-link {{if eq .Nav "agents"}}active{{end}}" href="/agents">{{icon "server"}}Agents</a>
     <a class="nav-link {{if eq .Nav "audit"}}active{{end}}" href="/audit">{{icon "activity"}}Audit log</a>
     <div class="sidebar-footer">
```

Modify `app/web/static/app.css` (apply this diff; context lines are the current code):

```diff
--- a/app/web/static/app.css
+++ b/app/web/static/app.css
@@ -128,6 +128,7 @@ p { margin: 0; }
 .nav-link:hover { background: var(--surface-2); color: var(--text); text-decoration: none; }
 .nav-link.active { background: var(--accent-soft); color: var(--accent-text); }
 .nav-link.active svg { opacity: 1; }
+.nav-count { margin-left: auto; }
 
 .sidebar-footer { margin-top: auto; padding-top: 12px; border-top: 1px solid var(--border); }
 .me { display: flex; align-items: center; gap: 10px; padding: 8px 6px; }
```

Modify `cmd/controlplane/modules/web.go` (apply this diff; context lines are the current code):

```diff
--- a/cmd/controlplane/modules/web.go
+++ b/cmd/controlplane/modules/web.go
@@ -1,6 +1,7 @@
 package modules
 
 import (
+	"github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
 	auditLookups "github.com/klzn-labs/kalzone/app/audit/repositories/lookups"
 	"github.com/klzn-labs/kalzone/app/audit/usecases/query"
 	"github.com/klzn-labs/kalzone/app/authz"
@@ -27,6 +28,7 @@ import (
 	peoplePage "github.com/klzn-labs/kalzone/app/web/handlers/people"
 	queuesPage "github.com/klzn-labs/kalzone/app/web/handlers/queues"
 	releasesPage "github.com/klzn-labs/kalzone/app/web/handlers/releases"
+	requestsPage "github.com/klzn-labs/kalzone/app/web/handlers/requests"
 	servicePage "github.com/klzn-labs/kalzone/app/web/handlers/service"
 	servicesPage "github.com/klzn-labs/kalzone/app/web/handlers/services"
 	teamPage "github.com/klzn-labs/kalzone/app/web/handlers/team"
@@ -43,7 +45,7 @@ func Web() fx.Option {
 			servicePage.NewServiceHandler, environmentPage.NewEnvironmentHandler, teamsPage.NewTeamsHandler,
 			teamPage.NewTeamHandler, auditPage.NewAuditHandler, peoplePage.NewPeopleHandler,
 			releasesPage.NewReleasesHandler, agentsPage.NewAgentsHandler, queuesPage.NewQueuesHandler,
-			tokensPage.NewTokensHandler,
+			tokensPage.NewTokensHandler, requestsPage.NewRequestsHandler,
 		),
 		viewInjections(),
 		pageInjections(),
@@ -121,6 +123,7 @@ func tokenPageInjections() fx.Option {
 	return fx.Provide(
 		func(u tokens.TokensUsecase) tokensPage.Tokens { return u },
 		func(u teams.TeamsUsecase) tokensPage.AllTeams { return u },
+		func(u requests.RequestsUsecase) requestsPage.Requests { return u },
 	)
 }
 
@@ -130,6 +133,6 @@ func pageRoutes() fx.Option {
 		servicesPage.RegisterServicesRoutes, servicePage.RegisterServiceRoutes, environmentPage.RegisterEnvironmentRoutes,
 		teamsPage.RegisterTeamsRoutes, teamPage.RegisterTeamRoutes, auditPage.RegisterAuditRoutes, peoplePage.RegisterPeopleRoutes,
 		releasesPage.RegisterReleasesRoutes, agentsPage.RegisterAgentsRoutes, queuesPage.RegisterQueuesRoutes,
-		tokensPage.RegisterTokensRoutes,
+		tokensPage.RegisterTokensRoutes, requestsPage.RegisterRequestsRoutes,
 	)
 }
```

- [ ] **Step 5: Run the tests**

Run: `go test -p 1 -count=1 ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: PASS, including every test in `internal/integration/tests/approvals_test.go` and the changed `Test_ProdNeedsProtectedAndApprovalUnlessBypassed`

- [ ] **Step 6: Commit**

```bash
git add internal pkg/config app/approvals cmd app/web
git commit -m "Ask for approval from the API and decide requests in the browser"
```

---

### Task 9: Docs — README

**Files:**
- Modify: `README.md`

Approvals section after Access tokens, the core bullet, the corrected protected-environment sentence and the configuration row. The infra change (spec section 11) is done separately in `infra-plateful`: pass `PLATFORM_APPROVALS_WEBHOOK_URL` from the tfvar `alert_webhook_url` into the controlplane secrets env.

**Interfaces:**
- Consumes: nothing.


- [ ] **Step 1: Implement**

Modify `README.md` (apply this diff; context lines are the current code):

````diff
--- a/README.md
+++ b/README.md
@@ -11,6 +11,7 @@ API on the same usecases. Design documents live in `docs/superpowers/specs`.
 
 - GitHub sign-in (GitHub App user OAuth), server-side sessions, CSRF protection
 - Access tokens for agents and scripts: Bearer tokens owned by a person, limited by scopes and teams
+- Approvals: an agent's change to a protected environment waits as a request until a human approves it
 - Teams with `member` and `maintainer` roles, platform admins
 - Service catalog, fixed `dev` and `prod` environments (`prod` is protected)
 - Config per service and environment as immutable, numbered revisions
@@ -80,6 +81,7 @@ as a callback URL.
 | `PLATFORM_DEPLOY_TIMEOUT` | `30m` | deployments an agent stopped reporting on are failed after this |
 | `PLATFORM_QUEUES_TOKEN` | | shared secret the queue gateway authenticates with; the gateway API answers 503 without it |
 | `PLATFORM_QUEUES_GATEWAY_URL` | `http://queued:8080` | gateway address handed to services as `PLATFORM_QUEUE_URL` |
+| `PLATFORM_APPROVALS_WEBHOOK_URL` | | Discord webhook told about every new change request; empty sends nothing |
 
 Never run production with the default secrets key.
 
@@ -100,8 +102,8 @@ explicitly. Revoking a token frees its name; a token made by another token never
 outlives it. Tokens expire after 1 to 365 days (90 by default) and are shown
 once; only a hash is stored.
 
-On a protected environment (`prod`) a token needs `protected`, and then gets
-`err_approval_required` unless a platform admin gave it bypass from the website.
+On a protected environment (`prod`) a token needs `protected`, and then its change
+waits for approval (see Approvals) unless a platform admin gave it bypass from the website.
 Tokens are refused on website pages and need no CSRF header. Every change made
 with a token shows in the audit log as "person via token".
 
@@ -112,6 +114,31 @@ with a token shows in the audit log as "person via token".
 | `POST /api/v1/tokens` | `{"name","scopes","teams","expires_in_days","bypass"}` → the token, once |
 | `DELETE /api/v1/tokens/:id` | revoke |
 
+## Approvals
+
+A token with `protected` but without bypass does not change a protected environment
+directly. Config revisions, secrets, deploys, promotions, rollbacks, config
+applications and queue create, update and delete answer `202`:
+
+```json
+{"kind": "change_request", "id": "…", "status": "pending", "summary": "deploy release #14 to prod",
+ "approve_url": "https://platform.kalz.one/requests/…", "poll": "/api/v1/requests/…", "expires_at": "…"}
+```
+
+A maintainer of the service's team or an admin, the token's owner included, opens
+**Requests** in the browser, sees the change against what runs now, and approves or
+rejects it. Approval runs the change as the token, re-applied on the current state; it
+fails with `err_stale` only when the same config key changed or another deployment
+went live in the meantime. A newer request for the same thing cancels the older one,
+and requests expire after 24 hours. Promote pins the release that was live when asked.
+Queue pause, resume and dead-letter commands still answer `err_approval_required`.
+
+| Endpoint | Purpose |
+|---|---|
+| `GET /api/v1/requests/:id` | `status` (`pending`, `approved`, `executed`, `failed`, `rejected`, `cancelled`, `expired`), `result`, `reason`, `error` |
+| `GET /api/v1/requests?status=pending` | the newest 100 in the token's teams |
+| `POST /api/v1/requests/:id/cancel` | withdraw your own request |
+
 ## Builds and releases
 
 1. Give the GitHub App the repository permission **Actions: read and write**,
````

- [ ] **Step 2: Run the tests**

Run: `git diff --stat`
Expected: only `README.md`

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Document approvals"
```

---

- [ ] **Check the branch shape**

Run: `git log --oneline -6`
Expected: on top of the spec and plan commits, four commits in the order storage, rules, handlers, docs.

---

## Self-review notes

- Spec §2 decisions: approvers (Tasks 2, 6, 8), Discord (Task 7), re-apply and conflicts (Tasks 3, 4, 5), secrets sealed at request (Task 3), supersede (Task 6), expiry (Tasks 1, 6). §3 table: kinds and subjects in Tasks 3–5; pause and dead letters stay refused (Task 5). §4 lifecycle and conditional transitions: Tasks 1, 6. §5 table and proposals: Tasks 1, 3–5. §6 gate and 202: Tasks 2, 7. §7 approval: Tasks 2, 6. §8: Task 6. §9 API: Task 7. §10 UI: Task 8. §11 notification: Task 7, infra in Task 9's note. §12 audit: Tasks 2–6, 8. §14 tests: every task. §17 done-when: `Test_AnAgentsProdDeployWaitsForAHuman` plus a manual check of Discord and the audit log after deploy.
- `queue.update` conflicts keep `err_conflict` from the existing `errStale`; changing its code would break `Test_Update_RefusesAChangeMadeOnAnOlderVersion` and gives agents nothing new.
- Requests are created only by tokens, so `TokenID` is never empty when `ActorOf` runs.
- After Task 6 the rules commit builds and its unit tests pass, but integration tests fail until Task 7 wires the `Requests` ports; the layers are reviewed together and the branch is green at the handlers commit.

# Managed queues, plan 1 of 6: control plane

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The control plane stores queues with versioned settings, serves them over its API with the usual authorization and audit, gives every deployed service its queue credentials, and serves the gateway's desired state, stats and commands, so plan 2's gateway has everything it needs.

**Architecture:** A new domain `app/queue` in the layered-hexagonal style of the other domains (entities → repositories/mysql → usecases → handlers), wired with fx in `cmd/controlplane/modules/queue.go`. The deploy domain gets the credentials through a consumer interface at claim time, like it gets config today. The gateway API mirrors the agent API: a shared bearer token, public routes under `/queued/v1`.

**Tech Stack:** Go 1.26, Gin, Uber fx, GORM on MySQL 8, golang-migrate, testify; integration tests in `internal/integration`.

**Spec:** `docs/superpowers/specs/2026-10-08-managed-queues-design.md` (§3, §5 auth part, §6, §7, §12 delete rule). Plans 2–6 cover the gateway, infra, library, UI and Plateful.

## Global Constraints

- Function bodies ≤ 20 lines, at most 2 levels of nesting, **no comments** in any file (user rule; a hook enforces it).
- One commit per layer, bottom-up: storage, rules, handlers + wiring + integration tests. No fix/wip commits; no `Co-Authored-By`.
- Queue name: `^[a-z][a-z0-9-]{1,62}$`, unique per environment.
- Defaults: rate 10/s, in flight 4, attempts 5, exponential backoff 1s → 5min, delivery timeout 30s, retention 7 days, max message 256 KiB.
- Limits: rate 0.1–1000; in flight 1–100; attempts 1–50; backoff `fixed|exponential`, initial 100ms–1h, max ≥ initial and ≤ 6h; delivery timeout 1s–15min; retention 1h–30 days (720h); max message 1 KiB–1 MiB; path starts with `/`, ≤ 255 chars.
- Reserved config names: `PLATFORM_QUEUE_URL` (variable), `PLATFORM_QUEUE_TOKEN`, `PLATFORM_QUEUE_SIGNING_KEY` (secrets).
- Editing a queue follows `authz.EditConfig` for the consumer's team and the queue's environment (members edit dev, maintainers prod).
- A queue holding messages cannot be deleted without `force`.
- Gateway auth: `Authorization: Bearer <PLATFORM_QUEUED_TOKEN>`; no token configured → 503 `err_not_configured`.
- Stats older than 60 seconds are shown as no data.
- Previous signing key stays valid 10 minutes after rotation.

## Review Focus

1. A publisher or consumer service that is later deleted: listing and desired state must skip it, not fail. Test in Task 4 (`Test_Desired_SkipsDeletedServices`).
2. Two people editing the same queue at once: the second update must be a 409, not a lost write. Test in Task 2 (`Test_InsertVersion_SameNumberTwiceIsAConflict`).
3. A team trying to set `PLATFORM_QUEUE_TOKEN` by hand through the config API: refused with 400. Test in Task 5.
4. The gateway asking for desired state before any queue exists: an empty list with 200, not 404. Test in Task 6.
5. Deleting a queue whose last stats are stale (gateway silent): the depth is unknown, so deletion without `force` is refused. Test in Task 3 (`Test_Delete_RefusesUnknownDepthWithoutForce`).

## File Structure

| File | Responsibility |
|---|---|
| `migrations/000004_queues.up.sql` / `.down.sql` | five tables |
| `internal/testdb/testdb.go` | add the new tables to `tables` |
| `app/queue/entities/queue.go` | `Queue`, `Settings`, `Backoff`, `Version`, defaults |
| `app/queue/entities/validate.go` | settings and name validation |
| `app/queue/entities/credentials.go` | `Credentials`, token and key generation, hashing |
| `app/queue/entities/stats.go` | `Stats`, `DeadLetter`, `Command` |
| `app/queue/repositories/mysql/model.go`, `queues.go`, `credentials.go`, `commands.go` | storage |
| `app/queue/repositories/catalog/catalog.go` | service and environment lookup adapter |
| `app/queue/usecases/queues/queues.go`, `edit.go`, `delete.go`, `dead.go` | queue CRUD, pause, dead letter commands |
| `app/queue/usecases/credentials/credentials.go` | ensure and rotate credentials |
| `app/queue/usecases/gateway/gateway.go`, `statsstore.go` | desired state, stats intake, command completion |
| `app/queue/handlers/queues/queues.go`, `dto.go` | public API |
| `app/queue/handlers/gatewayapi/gatewayapi.go` | `/queued/v1` |
| `app/deploy/repositories/queue/queue.go` | deploy-side adapter for credentials |
| `app/deploy/usecases/agents/claim.go` | merge credentials into the workload |
| `app/envconfig/entities/validate.go` | reserved names |
| `pkg/config/config.go` | `Queues{GatewayURL, Token}` |
| `pkg/events/events.go` | `TargetQueue` |
| `cmd/controlplane/modules/queue.go`, `options/options.go` | wiring |
| `internal/integration/tests/queues_test.go` | end-to-end through HTTP |

---

### Task 1: Storage — migration, entities, repository

**Files:**
- Create: `migrations/000004_queues.up.sql`, `migrations/000004_queues.down.sql`
- Modify: `internal/testdb/testdb.go` (`tables`)
- Create: `app/queue/entities/queue.go`, `app/queue/entities/credentials.go`, `app/queue/entities/stats.go`
- Create: `app/queue/repositories/mysql/model.go`, `queues.go`, `credentials.go`, `commands.go`
- Test: `app/queue/repositories/mysql/queues_test.go`, `credentials_test.go`, `commands_test.go`

**Interfaces:**
- Produces (entities):

```go
type Backoff struct {
	Kind    string        `json:"kind"`
	Initial time.Duration `json:"initial"`
	Max     time.Duration `json:"max"`
}

type Settings struct {
	Path            string        `json:"path"`
	RatePerSecond   float64       `json:"rate_per_second"`
	MaxInFlight     int           `json:"max_in_flight"`
	MaxAttempts     int           `json:"max_attempts"`
	Backoff         Backoff       `json:"backoff"`
	DeliveryTimeout time.Duration `json:"delivery_timeout"`
	Retention       time.Duration `json:"retention"`
	MaxMessageBytes int           `json:"max_message_bytes"`
}

type Queue struct {
	ID, Name, Environment, TeamID, ConsumerID string
	Version     int
	Settings    Settings
	Publishers  []string
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Version struct {
	QueueID   string
	Number    int
	Settings  Settings
	AuthorID  string
	Message   string
	CreatedAt time.Time
}

func DefaultSettings(path string) Settings

type Credentials struct {
	ServiceID, Environment string
	TokenCiphertext, TokenNonce []byte
	TokenHash                   string
	KeyCiphertext, KeyNonce     []byte
	PreviousKeyCiphertext, PreviousKeyNonce []byte
	KeyID                       string
	RotatedAt                   *time.Time
}

type Command struct {
	ID, QueueID, Kind, Argument string
	CreatedAt time.Time
	DoneAt    *time.Time
}

const (
	CommandReplay    = "replay"
	CommandReplayAll = "replay_all"
	CommandPurgeDead = "purge_dead"
	CommandDelete    = "delete"
)
```

- Produces (repository `mysql.QueueRepository`):

```go
func NewQueueRepository(db *gorm.DB) QueueRepository
func (r QueueRepository) Create(ctx context.Context, q entities.Queue, v entities.Version) error
func (r QueueRepository) InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error
func (r QueueRepository) SetPaused(ctx context.Context, queueID string, paused bool) error
func (r QueueRepository) Get(ctx context.Context, environment, name string) (entities.Queue, error)
func (r QueueRepository) List(ctx context.Context, environment, teamID string) ([]entities.Queue, error)
func (r QueueRepository) All(ctx context.Context) ([]entities.Queue, error)
func (r QueueRepository) Delete(ctx context.Context, queueID string) error
func (r QueueRepository) Versions(ctx context.Context, queueID string, limit int) ([]entities.Version, error)
func (r QueueRepository) Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error)
func (r QueueRepository) SaveCredentials(ctx context.Context, c entities.Credentials) error
func (r QueueRepository) AllCredentials(ctx context.Context) ([]entities.Credentials, error)
func (r QueueRepository) AddCommand(ctx context.Context, c entities.Command) error
func (r QueueRepository) PendingCommands(ctx context.Context) ([]entities.Command, error)
func (r QueueRepository) FinishCommand(ctx context.Context, id string, at time.Time) error
```

`Get` returns `apperror.NotFound("queue not found")` when missing. `InsertVersion` returns `apperror.Conflict("the queue changed while you were editing it, reload and try again")` on a duplicate `(queue_id, number)`. `Delete` removes the queue, its versions, publishers and pending commands in one transaction, except a pending `delete` command, which the gateway still needs.

- [ ] **Step 1: Write the migration**

`migrations/000004_queues.up.sql`:

```sql
CREATE TABLE queues (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(63) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    team_id CHAR(36) NOT NULL,
    consumer_id CHAR(36) NOT NULL,
    version INT NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    UNIQUE KEY queues_name (environment, name),
    KEY queues_team (team_id)
);

CREATE TABLE queue_versions (
    queue_id CHAR(36) NOT NULL,
    number INT NOT NULL,
    settings JSON NOT NULL,
    author_id CHAR(36) NOT NULL,
    message VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (queue_id, number)
);

CREATE TABLE queue_publishers (
    queue_id CHAR(36) NOT NULL,
    service_id CHAR(36) NOT NULL,
    PRIMARY KEY (queue_id, service_id),
    KEY queue_publishers_service (service_id)
);

CREATE TABLE queue_credentials (
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    token_ciphertext VARBINARY(512) NOT NULL,
    token_nonce VARBINARY(32) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    key_ciphertext VARBINARY(512) NOT NULL,
    key_nonce VARBINARY(32) NOT NULL,
    previous_key_ciphertext VARBINARY(512) NULL,
    previous_key_nonce VARBINARY(32) NULL,
    key_id VARCHAR(32) NOT NULL,
    rotated_at DATETIME(3) NULL,
    PRIMARY KEY (service_id, environment),
    UNIQUE KEY queue_credentials_token (token_hash)
);

CREATE TABLE queue_commands (
    id CHAR(36) NOT NULL PRIMARY KEY,
    queue_id CHAR(36) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    argument VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    done_at DATETIME(3) NULL,
    KEY queue_commands_pending (done_at, created_at)
);
```

`migrations/000004_queues.down.sql`:

```sql
DROP TABLE queue_commands;
DROP TABLE queue_credentials;
DROP TABLE queue_publishers;
DROP TABLE queue_versions;
DROP TABLE queues;
```

In `internal/testdb/testdb.go` append `"queues", "queue_versions", "queue_publishers", "queue_credentials", "queue_commands"` to `tables`.

- [ ] **Step 2: Write the entities**

`app/queue/entities/queue.go`:

```go
package entities

import "time"

const (
	BackoffFixed       = "fixed"
	BackoffExponential = "exponential"
)

type Backoff struct {
	Kind    string        `json:"kind"`
	Initial time.Duration `json:"initial"`
	Max     time.Duration `json:"max"`
}

type Settings struct {
	Path            string        `json:"path"`
	RatePerSecond   float64       `json:"rate_per_second"`
	MaxInFlight     int           `json:"max_in_flight"`
	MaxAttempts     int           `json:"max_attempts"`
	Backoff         Backoff       `json:"backoff"`
	DeliveryTimeout time.Duration `json:"delivery_timeout"`
	Retention       time.Duration `json:"retention"`
	MaxMessageBytes int           `json:"max_message_bytes"`
}

type Queue struct {
	ID          string
	Name        string
	Environment string
	TeamID      string
	ConsumerID  string
	Version     int
	Settings    Settings
	Publishers  []string
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Version struct {
	QueueID   string
	Number    int
	Settings  Settings
	AuthorID  string
	Message   string
	CreatedAt time.Time
}

func DefaultSettings(path string) Settings {
	return Settings{
		Path: path, RatePerSecond: 10, MaxInFlight: 4, MaxAttempts: 5,
		Backoff:         Backoff{Kind: BackoffExponential, Initial: time.Second, Max: 5 * time.Minute},
		DeliveryTimeout: 30 * time.Second, Retention: 7 * 24 * time.Hour, MaxMessageBytes: 256 << 10,
	}
}
```

`app/queue/entities/credentials.go`:

```go
package entities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const KeyGrace = 10 * time.Minute

type Credentials struct {
	ServiceID             string
	Environment           string
	TokenCiphertext       []byte
	TokenNonce            []byte
	TokenHash             string
	KeyCiphertext         []byte
	KeyNonce              []byte
	PreviousKeyCiphertext []byte
	PreviousKeyNonce      []byte
	KeyID                 string
	RotatedAt             *time.Time
}

func NewSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (c Credentials) PreviousKeyValid(now time.Time) bool {
	return c.RotatedAt != nil && len(c.PreviousKeyCiphertext) > 0 && now.Sub(*c.RotatedAt) < KeyGrace
}
```

`app/queue/entities/stats.go`:

```go
package entities

import "time"

const (
	CommandReplay    = "replay"
	CommandReplayAll = "replay_all"
	CommandPurgeDead = "purge_dead"
	CommandDelete    = "delete"
)

type Command struct {
	ID        string
	QueueID   string
	Kind      string
	Argument  string
	CreatedAt time.Time
	DoneAt    *time.Time
}

type DeadLetter struct {
	ID          string    `json:"id"`
	Attempts    int       `json:"attempts"`
	LastStatus  int       `json:"last_status"`
	LastError   string    `json:"last_error"`
	PublishedAt time.Time `json:"published_at"`
	BodyPreview string    `json:"body_preview"`
}

type Stats struct {
	QueueID            string       `json:"queue_id"`
	Depth              int64        `json:"depth"`
	InFlight           int          `json:"in_flight"`
	OldestAgeSeconds   float64      `json:"oldest_age_seconds"`
	DeliveredPerMinute float64      `json:"delivered_per_minute"`
	FailedPerMinute    float64      `json:"failed_per_minute"`
	DeadLetters        int64        `json:"dead_letters"`
	NewestDead         []DeadLetter `json:"newest_dead"`
	ReportedAt         time.Time    `json:"reported_at"`
}

const StatsFreshFor = 60 * time.Second

func (s Stats) Fresh(now time.Time) bool {
	return !s.ReportedAt.IsZero() && now.Sub(s.ReportedAt) <= StatsFreshFor
}
```

- [ ] **Step 3: Write the failing repository tests**

`app/queue/repositories/mysql/queues_test.go`:

```go
package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var at = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func queue(id, env, name string) (entities.Queue, entities.Version) {
	settings := entities.DefaultSettings("/hooks/" + name)
	q := entities.Queue{
		ID: id, Name: name, Environment: env, TeamID: "team", ConsumerID: "consumer", Version: 1,
		Settings: settings, Publishers: []string{"consumer", "importer"}, CreatedAt: at, UpdatedAt: at,
	}
	return q, entities.Version{QueueID: id, Number: 1, Settings: settings, AuthorID: "ana", CreatedAt: at}
}

func Test_Create_GetReturnsSettingsAndPublishers(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))

	got, err := repo.Get(ctx, "prod", "place-import")

	require.NoError(t, err)
	require.Equal(t, q.Settings, got.Settings)
	require.ElementsMatch(t, []string{"consumer", "importer"}, got.Publishers)
	require.Equal(t, 1, got.Version)
}

func Test_Get_MissingIsNotFound(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))

	_, err := repo.Get(ctx, "prod", "nope")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_SameNameInAnotherEnvironmentIsAllowed(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	prod, pv := queue("q1", "prod", "place-import")
	dev, dv := queue("q2", "dev", "place-import")
	again, av := queue("q3", "prod", "place-import")

	require.NoError(t, repo.Create(ctx, prod, pv))
	require.NoError(t, repo.Create(ctx, dev, dv))
	require.ErrorIs(t, repo.Create(ctx, again, av), apperror.Conflict(""))
}

func Test_InsertVersion_UpdatesTheQueueAndKeepsHistory(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	q.Version, q.Settings.MaxAttempts, q.Publishers = 2, 9, []string{"consumer"}

	require.NoError(t, repo.InsertVersion(ctx, q, entities.Version{QueueID: "q1", Number: 2, Settings: q.Settings, AuthorID: "ana", CreatedAt: at}))

	got, _ := repo.Get(ctx, "prod", "place-import")
	history, err := repo.Versions(ctx, "q1", 10)
	require.NoError(t, err)
	require.Equal(t, 9, got.Settings.MaxAttempts)
	require.Equal(t, []string{"consumer"}, got.Publishers)
	require.Len(t, history, 2)
	require.Equal(t, 2, history[0].Number)
}

func Test_InsertVersion_SameNumberTwiceIsAConflict(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	q.Version = 2
	next := entities.Version{QueueID: "q1", Number: 2, Settings: q.Settings, AuthorID: "ana", CreatedAt: at}
	require.NoError(t, repo.InsertVersion(ctx, q, next))

	err := repo.InsertVersion(ctx, q, next)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_List_FiltersByEnvironmentAndTeam(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	a, av := queue("q1", "prod", "a")
	b, bv := queue("q2", "dev", "b")
	c, cv := queue("q3", "prod", "c")
	c.TeamID = "other"
	for _, pair := range []struct {
		q entities.Queue
		v entities.Version
	}{{a, av}, {b, bv}, {c, cv}} {
		require.NoError(t, repo.Create(ctx, pair.q, pair.v))
	}

	prod, _ := repo.List(ctx, "prod", "")
	team, _ := repo.List(ctx, "", "team")
	all, err := repo.All(ctx)

	require.NoError(t, err)
	require.Len(t, prod, 2)
	require.Len(t, team, 2)
	require.Len(t, all, 3)
}

func Test_Delete_RemovesVersionsPublishersAndCommands(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c1", QueueID: "q1", Kind: entities.CommandReplayAll, CreatedAt: at}))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c2", QueueID: "q1", Kind: entities.CommandDelete, CreatedAt: at}))

	require.NoError(t, repo.Delete(ctx, "q1"))

	_, err := repo.Get(ctx, "prod", "place-import")
	pending, _ := repo.PendingCommands(ctx)
	versions, _ := repo.Versions(ctx, "q1", 10)
	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Len(t, pending, 1)
	require.Equal(t, "c2", pending[0].ID)
	require.Empty(t, versions)
}

func Test_SetPaused(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))

	require.NoError(t, repo.SetPaused(ctx, "q1", true))

	got, _ := repo.Get(ctx, "prod", "place-import")
	require.True(t, got.Paused)
}
```

`app/queue/repositories/mysql/credentials_test.go`:

```go
package mysql_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Credentials_SaveReplacesAndReads(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	c := entities.Credentials{
		ServiceID: "svc", Environment: "prod", TokenCiphertext: []byte("t"), TokenNonce: []byte("n"),
		TokenHash: entities.HashToken("token"), KeyCiphertext: []byte("k"), KeyNonce: []byte("n"), KeyID: "k1",
	}
	require.NoError(t, repo.SaveCredentials(ctx, c))
	c.PreviousKeyCiphertext, c.PreviousKeyNonce, c.RotatedAt = []byte("k"), []byte("n"), &at
	c.KeyCiphertext = []byte("k2")
	require.NoError(t, repo.SaveCredentials(ctx, c))

	got, found, err := repo.Credentials(ctx, "svc", "prod")
	_, missing, _ := repo.Credentials(ctx, "svc", "dev")
	all, _ := repo.AllCredentials(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.False(t, missing)
	require.Equal(t, []byte("k2"), got.KeyCiphertext)
	require.Equal(t, []byte("k"), got.PreviousKeyCiphertext)
	require.Len(t, all, 1)
}
```

`app/queue/repositories/mysql/commands_test.go`:

```go
package mysql_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Commands_PendingUntilFinished(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c1", QueueID: "q1", Kind: entities.CommandReplay, Argument: "m1", CreatedAt: at}))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c2", QueueID: "q1", Kind: entities.CommandPurgeDead, CreatedAt: at}))

	require.NoError(t, repo.FinishCommand(ctx, "c1", at))

	pending, err := repo.PendingCommands(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "c2", pending[0].ID)
}
```

- [ ] **Step 4: Run the tests to see them fail**

Run: `go test ./app/queue/...`
Expected: build failure, `undefined: mysql.NewQueueRepository`.

- [ ] **Step 5: Write the models**

`app/queue/repositories/mysql/model.go`:

```go
package mysql

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

type queueModel struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Environment string
	TeamID      string
	ConsumerID  string
	Version     int
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (queueModel) TableName() string { return "queues" }

type versionModel struct {
	QueueID   string `gorm:"primaryKey"`
	Number    int    `gorm:"primaryKey"`
	Settings  []byte
	AuthorID  string
	Message   string
	CreatedAt time.Time
}

func (versionModel) TableName() string { return "queue_versions" }

type publisherModel struct {
	QueueID   string `gorm:"primaryKey"`
	ServiceID string `gorm:"primaryKey"`
}

func (publisherModel) TableName() string { return "queue_publishers" }

type credentialsModel struct {
	ServiceID             string `gorm:"primaryKey"`
	Environment           string `gorm:"primaryKey"`
	TokenCiphertext       []byte
	TokenNonce            []byte
	TokenHash             string
	KeyCiphertext         []byte
	KeyNonce              []byte
	PreviousKeyCiphertext []byte
	PreviousKeyNonce      []byte
	KeyID                 string
	RotatedAt             *time.Time
}

func (credentialsModel) TableName() string { return "queue_credentials" }

type commandModel struct {
	ID        string `gorm:"primaryKey"`
	QueueID   string
	Kind      string
	Argument  string
	CreatedAt time.Time
	DoneAt    *time.Time
}

func (commandModel) TableName() string { return "queue_commands" }

func fromQueue(q entities.Queue) queueModel {
	return queueModel{
		ID: q.ID, Name: q.Name, Environment: q.Environment, TeamID: q.TeamID, ConsumerID: q.ConsumerID,
		Version: q.Version, Paused: q.Paused, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
}

func fromVersion(v entities.Version) (versionModel, error) {
	raw, err := json.Marshal(v.Settings)
	return versionModel{
		QueueID: v.QueueID, Number: v.Number, Settings: raw, AuthorID: v.AuthorID,
		Message: v.Message, CreatedAt: v.CreatedAt,
	}, err
}

func (m versionModel) toEntity() (entities.Version, error) {
	v := entities.Version{QueueID: m.QueueID, Number: m.Number, AuthorID: m.AuthorID, Message: m.Message, CreatedAt: m.CreatedAt}
	return v, json.Unmarshal(m.Settings, &v.Settings)
}

func (m queueModel) toEntity(settings entities.Settings, publishers []string) entities.Queue {
	return entities.Queue{
		ID: m.ID, Name: m.Name, Environment: m.Environment, TeamID: m.TeamID, ConsumerID: m.ConsumerID,
		Version: m.Version, Settings: settings, Publishers: publishers, Paused: m.Paused,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func publisherModels(queueID string, services []string) []publisherModel {
	result := make([]publisherModel, 0, len(services))
	for _, id := range services {
		result = append(result, publisherModel{QueueID: queueID, ServiceID: id})
	}
	return result
}

func fromCredentials(c entities.Credentials) credentialsModel {
	return credentialsModel(c)
}

func (m credentialsModel) toEntity() entities.Credentials {
	return entities.Credentials(m)
}

func fromCommand(c entities.Command) commandModel {
	return commandModel(c)
}

func (m commandModel) toEntity() entities.Command {
	return entities.Command(m)
}
```

- [ ] **Step 6: Write the queue repository**

`app/queue/repositories/mysql/queues.go`:

```go
package mysql

import (
	"context"
	"errors"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errQueueNotFound = apperror.NotFound("queue not found")
	errNameTaken     = apperror.Conflict("a queue with this name already exists in this environment")
	errConcurrent    = apperror.Conflict("the queue changed while you were editing it, reload and try again")
)

type QueueRepository struct {
	db *gorm.DB
}

func NewQueueRepository(db *gorm.DB) QueueRepository {
	return QueueRepository{db: db}
}

func (r QueueRepository) Create(ctx context.Context, q entities.Queue, v entities.Version) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ptr(fromQueue(q))).Error; err != nil {
			return err
		}
		return writeVersion(tx, q, v)
	})
	if isDuplicate(err) {
		return errNameTaken
	}
	return err
}

func (r QueueRepository) InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := writeVersion(tx, q, v); err != nil {
			return err
		}
		return tx.Model(&queueModel{ID: q.ID}).Updates(map[string]any{"version": q.Version, "updated_at": q.UpdatedAt}).Error
	})
	if isDuplicate(err) {
		return errConcurrent
	}
	return err
}

func writeVersion(tx *gorm.DB, q entities.Queue, v entities.Version) error {
	model, err := fromVersion(v)
	if err != nil {
		return err
	}
	if err := tx.Create(&model).Error; err != nil {
		return err
	}
	if err := tx.Where("queue_id = ?", q.ID).Delete(&publisherModel{}).Error; err != nil {
		return err
	}
	return createPublishers(tx, q)
}

func createPublishers(tx *gorm.DB, q entities.Queue) error {
	if len(q.Publishers) == 0 {
		return nil
	}
	return tx.Create(publisherModels(q.ID, q.Publishers)).Error
}

func (r QueueRepository) SetPaused(ctx context.Context, queueID string, paused bool) error {
	return r.db.WithContext(ctx).Model(&queueModel{ID: queueID}).Update("paused", paused).Error
}

func (r QueueRepository) Get(ctx context.Context, environment, name string) (entities.Queue, error) {
	var model queueModel
	err := r.db.WithContext(ctx).Where("environment = ? AND name = ?", environment, name).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Queue{}, errQueueNotFound
	}
	if err != nil {
		return entities.Queue{}, err
	}
	return r.hydrate(ctx, model)
}

func (r QueueRepository) List(ctx context.Context, environment, teamID string) ([]entities.Queue, error) {
	query := r.db.WithContext(ctx).Order("environment, name")
	if environment != "" {
		query = query.Where("environment = ?", environment)
	}
	if teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}
	var models []queueModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return r.hydrateAll(ctx, models)
}

func (r QueueRepository) All(ctx context.Context) ([]entities.Queue, error) {
	return r.List(ctx, "", "")
}

func (r QueueRepository) Delete(ctx context.Context, queueID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&publisherModel{}, &versionModel{}} {
			if err := tx.Where("queue_id = ?", queueID).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("queue_id = ? AND kind <> ?", queueID, entities.CommandDelete).Delete(&commandModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&queueModel{ID: queueID}).Error
	})
}

func (r QueueRepository) Versions(ctx context.Context, queueID string, limit int) ([]entities.Version, error) {
	var models []versionModel
	err := r.db.WithContext(ctx).Where("queue_id = ?", queueID).Order("number DESC").Limit(limit).Find(&models).Error
	if err != nil {
		return nil, err
	}
	result := make([]entities.Version, 0, len(models))
	for _, m := range models {
		v, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func (r QueueRepository) hydrateAll(ctx context.Context, models []queueModel) ([]entities.Queue, error) {
	result := make([]entities.Queue, 0, len(models))
	for _, m := range models {
		q, err := r.hydrate(ctx, m)
		if err != nil {
			return nil, err
		}
		result = append(result, q)
	}
	return result, nil
}

func (r QueueRepository) hydrate(ctx context.Context, m queueModel) (entities.Queue, error) {
	var version versionModel
	err := r.db.WithContext(ctx).Where("queue_id = ? AND number = ?", m.ID, m.Version).Take(&version).Error
	if err != nil {
		return entities.Queue{}, err
	}
	v, err := version.toEntity()
	if err != nil {
		return entities.Queue{}, err
	}
	var publishers []string
	err = r.db.WithContext(ctx).Model(&publisherModel{}).Where("queue_id = ?", m.ID).Order("service_id").Pluck("service_id", &publishers).Error
	return m.toEntity(v.Settings, publishers), err
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}

func ptr[T any](v T) *T {
	return &v
}
```

- [ ] **Step 7: Write the credentials and commands repositories**

`app/queue/repositories/mysql/credentials.go`:

```go
package mysql

import (
	"context"
	"errors"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r QueueRepository) Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error) {
	var model credentialsModel
	err := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Credentials{}, false, nil
	}
	return model.toEntity(), err == nil, err
}

func (r QueueRepository) SaveCredentials(ctx context.Context, c entities.Credentials) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(ptr(fromCredentials(c))).Error
}

func (r QueueRepository) AllCredentials(ctx context.Context) ([]entities.Credentials, error) {
	var models []credentialsModel
	if err := r.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Credentials, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
```

`app/queue/repositories/mysql/commands.go`:

```go
package mysql

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

func (r QueueRepository) AddCommand(ctx context.Context, c entities.Command) error {
	return r.db.WithContext(ctx).Create(ptr(fromCommand(c))).Error
}

func (r QueueRepository) PendingCommands(ctx context.Context) ([]entities.Command, error) {
	var models []commandModel
	if err := r.db.WithContext(ctx).Where("done_at IS NULL").Order("created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Command, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func (r QueueRepository) FinishCommand(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&commandModel{ID: id}).Update("done_at", at).Error
}
```

- [ ] **Step 8: Run the tests**

Run: `go test -count=1 ./app/queue/... && make lint`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add migrations/000004_queues.* internal/testdb/testdb.go app/queue/entities app/queue/repositories/mysql
git commit -m "Store queues, their settings versions, publishers, credentials and commands"
```

(Commit body: why the five tables exist; the unique `(environment, name)`; credentials sealed like config secrets.)

---

### Task 2: Rules — validation

**Files:**
- Create: `app/queue/entities/validate.go`
- Test: `app/queue/entities/validate_test.go`

**Interfaces:**
- Produces:

```go
func ValidateName(name string) error
func (s Settings) Validate() error
func (b Backoff) Delay(attempt int) time.Duration
```

`Delay` is exported for plan 2's gateway to share the formula's definition; jitter is added by the gateway, not here.

- [ ] **Step 1: Write the failing tests**

`app/queue/entities/validate_test.go`:

```go
package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/stretchr/testify/require"
)

func Test_ValidateName(t *testing.T) {
	for _, ok := range []string{"place-import", "ab", "q1"} {
		require.NoError(t, entities.ValidateName(ok), ok)
	}
	for _, bad := range []string{"", "a", "Place", "1queue", "has_underscore", string(make([]byte, 64))} {
		require.Error(t, entities.ValidateName(bad), bad)
	}
}

func Test_Settings_DefaultsAreValid(t *testing.T) {
	require.NoError(t, entities.DefaultSettings("/hooks/x").Validate())
}

func Test_Settings_Limits(t *testing.T) {
	broken := map[string]func(s *entities.Settings){
		"path":               func(s *entities.Settings) { s.Path = "hooks" },
		"long path":          func(s *entities.Settings) { s.Path = "/" + string(make([]byte, 255)) },
		"rate low":           func(s *entities.Settings) { s.RatePerSecond = 0.05 },
		"rate high":          func(s *entities.Settings) { s.RatePerSecond = 1001 },
		"in flight":          func(s *entities.Settings) { s.MaxInFlight = 0 },
		"in flight high":     func(s *entities.Settings) { s.MaxInFlight = 101 },
		"attempts":           func(s *entities.Settings) { s.MaxAttempts = 51 },
		"backoff kind":       func(s *entities.Settings) { s.Backoff.Kind = "linear" },
		"backoff initial":    func(s *entities.Settings) { s.Backoff.Initial = 50 * time.Millisecond },
		"backoff max<init":   func(s *entities.Settings) { s.Backoff.Max = 500 * time.Millisecond },
		"backoff max high":   func(s *entities.Settings) { s.Backoff.Max = 7 * time.Hour },
		"timeout":            func(s *entities.Settings) { s.DeliveryTimeout = 16 * time.Minute },
		"retention low":      func(s *entities.Settings) { s.Retention = 30 * time.Minute },
		"retention high":     func(s *entities.Settings) { s.Retention = 721 * time.Hour },
		"message too small":  func(s *entities.Settings) { s.MaxMessageBytes = 512 },
		"message too large":  func(s *entities.Settings) { s.MaxMessageBytes = 2 << 20 },
	}
	for name, breakIt := range broken {
		s := entities.DefaultSettings("/hooks/x")
		breakIt(&s)
		require.Error(t, s.Validate(), name)
	}
}

func Test_Backoff_Delay(t *testing.T) {
	expo := entities.Backoff{Kind: entities.BackoffExponential, Initial: time.Second, Max: 10 * time.Second}
	fixed := entities.Backoff{Kind: entities.BackoffFixed, Initial: 3 * time.Second, Max: 3 * time.Second}

	require.Equal(t, time.Second, expo.Delay(1))
	require.Equal(t, 4*time.Second, expo.Delay(3))
	require.Equal(t, 10*time.Second, expo.Delay(10))
	require.Equal(t, 10*time.Second, expo.Delay(200))
	require.Equal(t, 3*time.Second, fixed.Delay(7))
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./app/queue/entities/`
Expected: build failure, `undefined: entities.ValidateName`.

- [ ] **Step 3: Write the validation**

`app/queue/entities/validate.go`:

```go
package entities

import (
	"regexp"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

type check struct {
	ok      bool
	message string
}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid("queue name must be 2-63 lowercase letters, digits or dashes, starting with a letter")
	}
	return nil
}

func (s Settings) Validate() error {
	for _, c := range append(s.deliveryChecks(), s.Backoff.checks()...) {
		if !c.ok {
			return apperror.Invalid(c.message)
		}
	}
	return nil
}

func (s Settings) deliveryChecks() []check {
	return []check{
		{strings.HasPrefix(s.Path, "/") && len(s.Path) <= 255, "path must start with / and be at most 255 characters"},
		{s.RatePerSecond >= 0.1 && s.RatePerSecond <= 1000, "rate must be between 0.1 and 1000 messages per second"},
		{s.MaxInFlight >= 1 && s.MaxInFlight <= 100, "in flight must be between 1 and 100"},
		{s.MaxAttempts >= 1 && s.MaxAttempts <= 50, "attempts must be between 1 and 50"},
		{s.DeliveryTimeout >= time.Second && s.DeliveryTimeout <= 15*time.Minute, "delivery timeout must be between 1s and 15m"},
		{s.Retention >= time.Hour && s.Retention <= 720*time.Hour, "retention must be between 1 hour and 30 days"},
		{s.MaxMessageBytes >= 1<<10 && s.MaxMessageBytes <= 1<<20, "message size must be between 1 KiB and 1 MiB"},
	}
}

func (b Backoff) checks() []check {
	return []check{
		{b.Kind == BackoffFixed || b.Kind == BackoffExponential, "backoff must be fixed or exponential"},
		{b.Initial >= 100*time.Millisecond && b.Initial <= time.Hour, "backoff initial delay must be between 100ms and 1h"},
		{b.Max >= b.Initial && b.Max <= 6*time.Hour, "backoff max delay must be at least the initial delay and at most 6h"},
	}
}

func (b Backoff) Delay(attempt int) time.Duration {
	if b.Kind == BackoffFixed || attempt <= 1 {
		return min(b.Initial, b.Max)
	}
	delay := b.Initial
	for i := 1; i < attempt && delay < b.Max; i++ {
		delay *= 2
	}
	return min(delay, b.Max)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./app/queue/entities/ && make lint`
Expected: PASS.

- [ ] **Step 5: Hold the commit** — Task 3's use cases join this commit as the rules layer.

---

### Task 3: Rules — queue use cases (create, edit, pause, delete, dead letters)

**Files:**
- Create: `app/queue/usecases/queues/queues.go`, `edit.go`, `delete.go`, `dead.go`
- Create: `app/queue/repositories/catalog/catalog.go`
- Modify: `pkg/events/events.go` (add `TargetQueue = "queue"`)
- Test: `app/queue/usecases/queues/queues_test.go`, `fakes_test.go`

**Interfaces:**
- Consumes: Task 1 repository methods, Task 2 validation.
- Produces:

```go
type ServiceRef struct{ ID, Slug, OwnerTeamID string; Port int }

type Store interface {
	Create(ctx context.Context, q entities.Queue, v entities.Version) error
	InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error
	SetPaused(ctx context.Context, queueID string, paused bool) error
	Get(ctx context.Context, environment, name string) (entities.Queue, error)
	List(ctx context.Context, environment, teamID string) ([]entities.Queue, error)
	Delete(ctx context.Context, queueID string) error
	Versions(ctx context.Context, queueID string, limit int) ([]entities.Version, error)
	AddCommand(ctx context.Context, c entities.Command) error
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (queues.ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (queues.ServiceRef, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type StatsReader interface {
	Latest(queueID string) (entities.Stats, bool)
}

type Draft struct {
	Name, Environment, Consumer string
	Settings   entities.Settings
	Publishers []string
	Message    string
}

type Change struct {
	Settings   *entities.Settings
	Publishers *[]string
	Message    string
}

func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher) QueuesUsecase
func (u QueuesUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (entities.Queue, error)
func (u QueuesUsecase) Update(ctx context.Context, a actor.Actor, environment, name string, c Change) (entities.Queue, error)
func (u QueuesUsecase) SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error)
func (u QueuesUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error
func (u QueuesUsecase) Get(ctx context.Context, environment, name string) (entities.Queue, error)
func (u QueuesUsecase) List(ctx context.Context, environment, teamSlug string) ([]entities.Queue, error)
func (u QueuesUsecase) Versions(ctx context.Context, environment, name string) ([]entities.Version, error)
func (u QueuesUsecase) Stats(ctx context.Context, environment, name string) (entities.Stats, bool, error)
func (u QueuesUsecase) Replay(ctx context.Context, a actor.Actor, environment, name, messageID string) error
func (u QueuesUsecase) ReplayAll(ctx context.Context, a actor.Actor, environment, name string) error
func (u QueuesUsecase) PurgeDead(ctx context.Context, a actor.Actor, environment, name string) error
```

`TeamLookup` is `FindTeam(ctx context.Context, slug string) (string, error)` returning the team id; `List` with a team slug uses it. Rules:

- `Create`: valid name, existing environment, consumer service exists; queue `TeamID` = consumer's owner team; `authz.EditConfig` on `{TeamID, Environment}`; settings default to `DefaultSettings(path)` when zero, otherwise validated; publishers are service slugs resolved to ids, the consumer is always included; event `queue.created`.
- `Update`: same authorization; version = current + 1; event `queue.updated` with `version`.
- `SetPaused`: same authorization; events `queue.paused` / `queue.resumed`.
- `Delete`: same authorization; without `force`, refuse with `apperror.Conflict("the queue still holds messages; empty it or delete with force")` when the latest stats are missing, stale, or show depth or dead letters above 0; records a `CommandDelete` command (the gateway removes the streams), then deletes the row; event `queue.deleted`.
- `Replay`/`ReplayAll`/`PurgeDead`: same authorization; add a command; events `queue.dead_replayed`, `queue.dead_replayed_all`, `queue.dead_purged`.

- [ ] **Step 1: Write the fakes**

`app/queue/usecases/queues/fakes_test.go`:

```go
package queues_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var ctx = context.Background()

var ana = actor.Actor{UserID: "u-ana", Login: "ana"}

type store struct {
	queues   map[string]entities.Queue
	versions []entities.Version
	commands []entities.Command
}

func key(env, name string) string { return env + "/" + name }

func (s *store) Create(_ context.Context, q entities.Queue, v entities.Version) error {
	if _, taken := s.queues[key(q.Environment, q.Name)]; taken {
		return apperror.Conflict("taken")
	}
	s.queues[key(q.Environment, q.Name)] = q
	s.versions = append(s.versions, v)
	return nil
}

func (s *store) InsertVersion(_ context.Context, q entities.Queue, v entities.Version) error {
	s.queues[key(q.Environment, q.Name)] = q
	s.versions = append(s.versions, v)
	return nil
}

func (s *store) SetPaused(_ context.Context, id string, paused bool) error {
	for k, q := range s.queues {
		if q.ID == id {
			q.Paused = paused
			s.queues[k] = q
		}
	}
	return nil
}

func (s *store) Get(_ context.Context, env, name string) (entities.Queue, error) {
	q, ok := s.queues[key(env, name)]
	if !ok {
		return entities.Queue{}, apperror.NotFound("queue not found")
	}
	return q, nil
}

func (s *store) List(context.Context, string, string) ([]entities.Queue, error) {
	result := []entities.Queue{}
	for _, q := range s.queues {
		result = append(result, q)
	}
	return result, nil
}

func (s *store) Delete(_ context.Context, id string) error {
	for k, q := range s.queues {
		if q.ID == id {
			delete(s.queues, k)
		}
	}
	return nil
}

func (s *store) Versions(context.Context, string, int) ([]entities.Version, error) {
	return s.versions, nil
}

func (s *store) AddCommand(_ context.Context, c entities.Command) error {
	s.commands = append(s.commands, c)
	return nil
}

var services = map[string]queues.ServiceRef{
	"worker":   {ID: "s-worker", Slug: "worker", OwnerTeamID: "t-plateful", Port: 8081},
	"importer": {ID: "s-importer", Slug: "importer", OwnerTeamID: "t-plateful", Port: 8082},
}

type lookup struct{}

func (lookup) FindService(_ context.Context, slug string) (queues.ServiceRef, error) {
	s, ok := services[slug]
	if !ok {
		return queues.ServiceRef{}, apperror.NotFound("service not found")
	}
	return s, nil
}

func (lookup) FindServiceByID(_ context.Context, id string) (queues.ServiceRef, error) {
	for _, s := range services {
		if s.ID == id {
			return s, nil
		}
	}
	return queues.ServiceRef{}, apperror.NotFound("service not found")
}

func (lookup) EnvironmentExists(_ context.Context, slug string) error {
	if slug == "dev" || slug == "prod" {
		return nil
	}
	return apperror.NotFound("environment not found")
}

func (lookup) FindTeam(_ context.Context, slug string) (string, error) {
	return "t-" + slug, nil
}

type stats map[string]entities.Stats

func (s stats) Latest(id string) (entities.Stats, bool) {
	st, ok := s[id]
	return st, ok
}

type policy struct{ deny bool }

func (p policy) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error {
	if p.deny {
		return apperror.Forbidden("no")
	}
	return nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) { r.actions = append(r.actions, e.Action) }

type fixture struct {
	usecase queues.QueuesUsecase
	store   *store
	stats   stats
	events  *recorder
}

func newFixture(t *testing.T, deny bool) fixture {
	t.Helper()
	s := &store{queues: map[string]entities.Queue{}}
	st := stats{}
	r := &recorder{}
	u := queues.NewQueuesUsecase(s, lookup{}, lookup{}, lookup{}, st, policy{deny: deny}, r)
	return fixture{usecase: u, store: s, stats: st, events: r}
}

func draft() queues.Draft {
	return queues.Draft{Name: "place-import", Environment: "prod", Consumer: "worker", Publishers: []string{"importer"}, Settings: entities.Settings{Path: "/hooks/place-import"}}
}

func fresh(depth int64) entities.Stats {
	return entities.Stats{Depth: depth, ReportedAt: time.Now().UTC()}
}
```

- [ ] **Step 2: Write the failing tests**

`app/queue/usecases/queues/queues_test.go`:

```go
package queues_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_Create_FillsDefaultsOwnerAndConsumerAsPublisher(t *testing.T) {
	f := newFixture(t, false)

	q, err := f.usecase.Create(ctx, ana, draft())

	require.NoError(t, err)
	require.Equal(t, "t-plateful", q.TeamID)
	require.Equal(t, "s-worker", q.ConsumerID)
	require.Equal(t, entities.DefaultSettings("/hooks/place-import"), q.Settings)
	require.ElementsMatch(t, []string{"s-worker", "s-importer"}, q.Publishers)
	require.Equal(t, 1, q.Version)
	require.Equal(t, []string{"queue.created"}, f.events.actions)
}

func Test_Create_RefusesBadInputAndOutsiders(t *testing.T) {
	f := newFixture(t, false)
	denied := newFixture(t, true)
	badName, badEnv, badConsumer, badPublisher, badSettings := draft(), draft(), draft(), draft(), draft()
	badName.Name = "Bad"
	badEnv.Environment = "staging"
	badConsumer.Consumer = "ghost"
	badPublisher.Publishers = []string{"ghost"}
	badSettings.Settings.MaxInFlight = 500

	_, e1 := f.usecase.Create(ctx, ana, badName)
	_, e2 := f.usecase.Create(ctx, ana, badEnv)
	_, e3 := f.usecase.Create(ctx, ana, badConsumer)
	_, e4 := f.usecase.Create(ctx, ana, badPublisher)
	_, e5 := f.usecase.Create(ctx, ana, badSettings)
	_, e6 := denied.usecase.Create(ctx, ana, draft())

	require.ErrorIs(t, e1, apperror.Invalid(""))
	require.ErrorIs(t, e2, apperror.NotFound(""))
	require.ErrorIs(t, e3, apperror.NotFound(""))
	require.ErrorIs(t, e4, apperror.NotFound(""))
	require.ErrorIs(t, e5, apperror.Invalid(""))
	require.ErrorIs(t, e6, apperror.Forbidden(""))
	require.Empty(t, f.store.queues)
}

func Test_Update_BumpsTheVersion(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	settings := created.Settings
	settings.MaxAttempts = 9

	q, err := f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &settings, Message: "more retries"})

	require.NoError(t, err)
	require.Equal(t, 2, q.Version)
	require.Equal(t, 9, q.Settings.MaxAttempts)
	require.Len(t, f.store.versions, 2)
	require.Equal(t, "more retries", f.store.versions[1].Message)
	require.Equal(t, []string{"queue.created", "queue.updated"}, f.events.actions)
}

func Test_SetPaused(t *testing.T) {
	f := newFixture(t, false)
	f.usecase.Create(ctx, ana, draft())

	paused, err := f.usecase.SetPaused(ctx, ana, "prod", "place-import", true)

	require.NoError(t, err)
	require.True(t, paused.Paused)
	require.Contains(t, f.events.actions, "queue.paused")
}

func Test_Delete_RefusesAQueueWithMessagesUnlessForced(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = fresh(3)

	held := f.usecase.Delete(ctx, ana, "prod", "place-import", false)
	forced := f.usecase.Delete(ctx, ana, "prod", "place-import", true)

	require.ErrorIs(t, held, apperror.Conflict(""))
	require.NoError(t, forced)
	require.Empty(t, f.store.queues)
	require.Equal(t, entities.CommandDelete, f.store.commands[0].Kind)
}

func Test_Delete_RefusesUnknownDepthWithoutForce(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = entities.Stats{Depth: 0, ReportedAt: time.Now().Add(-2 * time.Minute)}

	err := f.usecase.Delete(ctx, ana, "prod", "place-import", false)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_Delete_AnEmptyQueue(t *testing.T) {
	f := newFixture(t, false)
	q, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[q.ID] = fresh(0)

	require.NoError(t, f.usecase.Delete(ctx, ana, "prod", "place-import", false))
}

func Test_DeadLetterCommands(t *testing.T) {
	f := newFixture(t, false)
	f.usecase.Create(ctx, ana, draft())

	require.NoError(t, f.usecase.Replay(ctx, ana, "prod", "place-import", "m1"))
	require.NoError(t, f.usecase.ReplayAll(ctx, ana, "prod", "place-import"))
	require.NoError(t, f.usecase.PurgeDead(ctx, ana, "prod", "place-import"))

	kinds := []string{}
	for _, c := range f.store.commands {
		kinds = append(kinds, c.Kind)
	}
	require.Equal(t, []string{entities.CommandReplay, entities.CommandReplayAll, entities.CommandPurgeDead}, kinds)
	require.Equal(t, "m1", f.store.commands[0].Argument)
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `go test ./app/queue/usecases/...`
Expected: build failure, package `queues` missing.

- [ ] **Step 4: Write the use case core**

`app/queue/usecases/queues/queues.go`:

```go
package queues

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type ServiceRef struct {
	ID          string
	Slug        string
	OwnerTeamID string
	Port        int
}

type Store interface {
	Create(ctx context.Context, q entities.Queue, v entities.Version) error
	InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error
	SetPaused(ctx context.Context, queueID string, paused bool) error
	Get(ctx context.Context, environment, name string) (entities.Queue, error)
	List(ctx context.Context, environment, teamID string) ([]entities.Queue, error)
	Delete(ctx context.Context, queueID string) error
	Versions(ctx context.Context, queueID string, limit int) ([]entities.Version, error)
	AddCommand(ctx context.Context, c entities.Command) error
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (ServiceRef, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type TeamLookup interface {
	FindTeam(ctx context.Context, slug string) (string, error)
}

type StatsReader interface {
	Latest(queueID string) (entities.Stats, bool)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type QueuesUsecase struct {
	store        Store
	services     ServiceLookup
	environments EnvironmentLookup
	teams        TeamLookup
	stats        StatsReader
	authz        Authorizer
	publisher    Publisher
}

func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher) QueuesUsecase {
	return QueuesUsecase{
		store: store, services: services, environments: environments, teams: teams,
		stats: stats, authz: authz, publisher: publisher,
	}
}

func (u QueuesUsecase) Get(ctx context.Context, environment, name string) (entities.Queue, error) {
	return u.store.Get(ctx, environment, name)
}

func (u QueuesUsecase) List(ctx context.Context, environment, teamSlug string) ([]entities.Queue, error) {
	if teamSlug == "" {
		return u.store.List(ctx, environment, "")
	}
	teamID, err := u.teams.FindTeam(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.store.List(ctx, environment, teamID)
}

func (u QueuesUsecase) Versions(ctx context.Context, environment, name string) ([]entities.Version, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return nil, err
	}
	return u.store.Versions(ctx, q.ID, 50)
}

func (u QueuesUsecase) Stats(ctx context.Context, environment, name string) (entities.Stats, bool, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Stats{}, false, err
	}
	st, ok := u.stats.Latest(q.ID)
	return st, ok, nil
}

func (u QueuesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	return q, u.authz.Can(ctx, a, authz.EditConfig, authz.Resource{TeamID: q.TeamID, Environment: q.Environment})
}

func (u QueuesUsecase) publish(ctx context.Context, a actor.Actor, action string, q entities.Queue, details map[string]any) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, Action: action, TargetKind: events.TargetQueue,
		TargetID: q.ID, TargetName: q.Name, TeamID: q.TeamID, Environment: q.Environment, Details: details,
	})
}
```

- [ ] **Step 5: Write create and update**

`app/queue/usecases/queues/edit.go`:

```go
package queues

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type Draft struct {
	Name        string
	Environment string
	Consumer    string
	Settings    entities.Settings
	Publishers  []string
	Message     string
}

type Change struct {
	Settings   *entities.Settings
	Publishers *[]string
	Message    string
}

func (u QueuesUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (entities.Queue, error) {
	consumer, err := u.checkDraft(ctx, d)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.authz.Can(ctx, a, authz.EditConfig, authz.Resource{TeamID: consumer.OwnerTeamID, Environment: d.Environment}); err != nil {
		return entities.Queue{}, err
	}
	q, err := u.newQueue(ctx, d, consumer)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.Create(ctx, q, versionOf(a, q, d.Message)); err != nil {
		return entities.Queue{}, err
	}
	u.publish(ctx, a, "queue.created", q, map[string]any{"version": q.Version})
	return q, nil
}

func (u QueuesUsecase) checkDraft(ctx context.Context, d Draft) (ServiceRef, error) {
	if err := entities.ValidateName(d.Name); err != nil {
		return ServiceRef{}, err
	}
	if err := u.environments.EnvironmentExists(ctx, d.Environment); err != nil {
		return ServiceRef{}, err
	}
	return u.services.FindService(ctx, d.Consumer)
}

func (u QueuesUsecase) newQueue(ctx context.Context, d Draft, consumer ServiceRef) (entities.Queue, error) {
	settings := withDefaults(d.Settings)
	if err := settings.Validate(); err != nil {
		return entities.Queue{}, err
	}
	publishers, err := u.publisherIDs(ctx, consumer, d.Publishers)
	now := clock.Now().UTC()
	return entities.Queue{
		ID: uuid.NewUUID(), Name: d.Name, Environment: d.Environment, TeamID: consumer.OwnerTeamID,
		ConsumerID: consumer.ID, Version: 1, Settings: settings, Publishers: publishers, CreatedAt: now, UpdatedAt: now,
	}, err
}

func withDefaults(s entities.Settings) entities.Settings {
	if s == (entities.Settings{Path: s.Path}) {
		return entities.DefaultSettings(s.Path)
	}
	return s
}

func (u QueuesUsecase) publisherIDs(ctx context.Context, consumer ServiceRef, slugs []string) ([]string, error) {
	ids := []string{consumer.ID}
	for _, slug := range slugs {
		s, err := u.services.FindService(ctx, slug)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

func (u QueuesUsecase) Update(ctx context.Context, a actor.Actor, environment, name string, c Change) (entities.Queue, error) {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	next, err := u.apply(ctx, q, c)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.InsertVersion(ctx, next, versionOf(a, next, c.Message)); err != nil {
		return entities.Queue{}, err
	}
	u.publish(ctx, a, "queue.updated", next, map[string]any{"version": next.Version, "message": c.Message})
	return next, nil
}

func (u QueuesUsecase) apply(ctx context.Context, q entities.Queue, c Change) (entities.Queue, error) {
	if c.Settings != nil {
		q.Settings = *c.Settings
	}
	if err := q.Settings.Validate(); err != nil {
		return entities.Queue{}, err
	}
	if c.Publishers != nil {
		consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
		if err != nil {
			return entities.Queue{}, err
		}
		if q.Publishers, err = u.publisherIDs(ctx, consumer, *c.Publishers); err != nil {
			return entities.Queue{}, err
		}
	}
	q.Version, q.UpdatedAt = q.Version+1, clock.Now().UTC()
	return q, nil
}

func versionOf(a actor.Actor, q entities.Queue, message string) entities.Version {
	return entities.Version{QueueID: q.ID, Number: q.Version, Settings: q.Settings, AuthorID: a.UserID, Message: message, CreatedAt: q.UpdatedAt}
}
```

- [ ] **Step 6: Write pause, delete and dead letter commands**

`app/queue/usecases/queues/delete.go`:

```go
package queues

import (
	"context"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

var errNotEmpty = apperror.Conflict("the queue still holds messages; empty it or delete with force")

func (u QueuesUsecase) SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error) {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.SetPaused(ctx, q.ID, paused); err != nil {
		return entities.Queue{}, err
	}
	q.Paused = paused
	u.publish(ctx, a, map[bool]string{true: "queue.paused", false: "queue.resumed"}[paused], q, nil)
	return q, nil
}

func (u QueuesUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if !force && !u.knownEmpty(q.ID) {
		return errNotEmpty
	}
	if err := u.command(ctx, q, entities.CommandDelete, q.Environment+"/"+q.Name); err != nil {
		return err
	}
	if err := u.store.Delete(ctx, q.ID); err != nil {
		return err
	}
	u.publish(ctx, a, "queue.deleted", q, map[string]any{"forced": force})
	return nil
}

func (u QueuesUsecase) knownEmpty(queueID string) bool {
	st, ok := u.stats.Latest(queueID)
	return ok && st.Fresh(clock.Now().UTC()) && st.Depth == 0 && st.DeadLetters == 0
}

func (u QueuesUsecase) command(ctx context.Context, q entities.Queue, kind, argument string) error {
	return u.store.AddCommand(ctx, entities.Command{
		ID: uuid.NewUUID(), QueueID: q.ID, Kind: kind, Argument: argument, CreatedAt: clock.Now().UTC(),
	})
}
```

`app/queue/usecases/queues/dead.go`:

```go
package queues

import (
	"context"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

func (u QueuesUsecase) Replay(ctx context.Context, a actor.Actor, environment, name, messageID string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandReplay, messageID, "queue.dead_replayed")
}

func (u QueuesUsecase) ReplayAll(ctx context.Context, a actor.Actor, environment, name string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandReplayAll, "", "queue.dead_replayed_all")
}

func (u QueuesUsecase) PurgeDead(ctx context.Context, a actor.Actor, environment, name string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandPurgeDead, "", "queue.dead_purged")
}

func (u QueuesUsecase) deadCommand(ctx context.Context, a actor.Actor, environment, name, kind, argument, action string) error {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if err := u.command(ctx, q, kind, argument); err != nil {
		return err
	}
	u.publish(ctx, a, action, q, map[string]any{"message": argument})
	return nil
}
```

Add `TargetQueue = "queue"` to the target constants in `pkg/events/events.go`.

- [ ] **Step 7: Write the catalog adapter**

`app/queue/repositories/catalog/catalog.go` adapts the catalog and team use cases (same shape as `app/envconfig/repositories/catalog`):

```go
package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	GetByID(ctx context.Context, id string) (catalog.Service, error)
}

type EnvironmentFinder interface {
	Get(ctx context.Context, slug string) (catalog.Environment, error)
}

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
}

type PortReader interface {
	Port(ctx context.Context, serviceID, environment string) (int, error)
}

type Lookup struct {
	services     ServiceFinder
	environments EnvironmentFinder
	teams        TeamFinder
}

func NewLookup(services ServiceFinder, environments EnvironmentFinder, teams TeamFinder) Lookup {
	return Lookup{services: services, environments: environments, teams: teams}
}

func (l Lookup) FindService(ctx context.Context, slug string) (queues.ServiceRef, error) {
	s, err := l.services.Get(ctx, slug)
	return queues.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID}, err
}

func (l Lookup) FindServiceByID(ctx context.Context, id string) (queues.ServiceRef, error) {
	s, err := l.services.GetByID(ctx, id)
	return queues.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID}, err
}

func (l Lookup) EnvironmentExists(ctx context.Context, slug string) error {
	_, err := l.environments.Get(ctx, slug)
	return err
}

func (l Lookup) FindTeam(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}
```

Before writing it, check the exact method names on `services.ServicesUsecase`, `environments.EnvironmentsUsecase` and the teams use case (`grep -n "^func (u" app/catalog/usecases/services/*.go app/catalog/usecases/environments/*.go app/team/usecases/teams/*.go`) and use those names in the interfaces; keep `PortReader` out if no caller needs it yet (the gateway's target port comes from the resolved config in Task 4).

- [ ] **Step 8: Run the tests**

Run: `go test ./app/queue/... && make lint`
Expected: PASS.

- [ ] **Step 9: Commit (rules layer: Tasks 2 and 3)**

```bash
git add app/queue/entities/validate*.go app/queue/usecases/queues app/queue/repositories/catalog pkg/events/events.go
git commit -m "Decide what a queue may be set to and who may change it"
```

---

### Task 4: Rules — credentials and the gateway's view

**Files:**
- Create: `app/queue/usecases/credentials/credentials.go`
- Create: `app/queue/usecases/gateway/gateway.go`, `statsstore.go`
- Test: `app/queue/usecases/credentials/credentials_test.go`, `app/queue/usecases/gateway/gateway_test.go`

**Interfaces:**
- Produces:

```go
package credentials

type Store interface {
	Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error)
	SaveCredentials(ctx context.Context, c entities.Credentials) error
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
	Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error)
}

type Opened struct {
	Token       string
	SigningKey  string
	PreviousKey string
}

func NewCredentialsUsecase(store Store, sealer Sealer) CredentialsUsecase
func (u CredentialsUsecase) Ensure(ctx context.Context, serviceID, environment string) (Opened, error)
func (u CredentialsUsecase) Rotate(ctx context.Context, serviceID, environment string) (Opened, error)
func (u CredentialsUsecase) Open(c entities.Credentials) (Opened, error)
```

Additional data for sealing: `"queue|" + serviceID + "|" + environment + "|token"` and `"...|key"` (and `"...|previous"`).

```go
package gateway

type QueueSource interface {
	All(ctx context.Context) ([]entities.Queue, error)
	PendingCommands(ctx context.Context) ([]entities.Command, error)
	FinishCommand(ctx context.Context, id string, at time.Time) error
	AllCredentials(ctx context.Context) ([]entities.Credentials, error)
}

type ServiceLookup interface {
	FindServiceByID(ctx context.Context, id string) (queues.ServiceRef, error)
}

type PortLookup interface {
	Port(ctx context.Context, serviceID, environment string) (int, error)
}

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type DesiredQueue struct {
	ID, Name, Environment string
	Version          int
	Settings         entities.Settings
	Paused           bool
	Target           string
	PublisherHashes  []string
	SigningKey       string
	PreviousKey      string
}

type Desired struct {
	Queues   []DesiredQueue
	Commands []entities.Command
}

func NewStatsStore() *StatsStore
func (s *StatsStore) Put(stats []entities.Stats, at time.Time)
func (s *StatsStore) Latest(queueID string) (entities.Stats, bool)

func NewGatewayUsecase(queues QueueSource, services ServiceLookup, ports PortLookup, creds Credentials, stats *StatsStore) GatewayUsecase
func (u GatewayUsecase) Desired(ctx context.Context) (Desired, error)
func (u GatewayUsecase) Report(ctx context.Context, stats []entities.Stats) error
func (u GatewayUsecase) Done(ctx context.Context, commandID string) error
```

`Target` is `http://<consumer-slug>-<env>:<port><path>`. `PortLookup.Port` returns the port of the consumer's current config revision in that environment (the envconfig `Current` spec port); a consumer with no saved config uses the default port 8080. A queue whose consumer service no longer exists is left out of `Desired`. Publisher hashes are the `TokenHash` of each publisher's credentials in that environment, read from `AllCredentials` without opening anything; publishers never deployed have no credentials and are left out. The consumer's signing key comes from `Credentials.Ensure`, which generates credentials on first use.

- [ ] **Step 1: Write the failing credentials tests**

`app/queue/usecases/credentials/credentials_test.go`:

```go
package credentials_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type store struct{ saved map[string]entities.Credentials }

func (s *store) Credentials(_ context.Context, serviceID, env string) (entities.Credentials, bool, error) {
	c, ok := s.saved[serviceID+env]
	return c, ok, nil
}

func (s *store) SaveCredentials(_ context.Context, c entities.Credentials) error {
	s.saved[c.ServiceID+c.Environment] = c
	return nil
}

func newUsecase(t *testing.T) (credentials.CredentialsUsecase, *store) {
	t.Helper()
	box, err := secretbox.New("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=", "k1")
	require.NoError(t, err)
	s := &store{saved: map[string]entities.Credentials{}}
	return credentials.NewCredentialsUsecase(s, box), s
}

func Test_Ensure_GeneratesOnceAndSealsAtRest(t *testing.T) {
	u, s := newUsecase(t)

	first, err := u.Ensure(ctx, "svc", "prod")
	second, _ := u.Ensure(ctx, "svc", "prod")

	require.NoError(t, err)
	require.Len(t, first.Token, 64)
	require.Len(t, first.SigningKey, 64)
	require.Equal(t, first, second)
	stored := s.saved["svcprod"]
	require.Equal(t, entities.HashToken(first.Token), stored.TokenHash)
	require.NotContains(t, string(stored.TokenCiphertext), first.Token)
}

func Test_Rotate_KeepsThePreviousKeyForTheGracePeriod(t *testing.T) {
	u, _ := newUsecase(t)
	before, _ := u.Ensure(ctx, "svc", "prod")

	after, err := u.Rotate(ctx, "svc", "prod")

	require.NoError(t, err)
	require.NotEqual(t, before.Token, after.Token)
	require.NotEqual(t, before.SigningKey, after.SigningKey)
	require.Equal(t, before.SigningKey, after.PreviousKey)
	clock.Set(t, time.Now().Add(entities.KeyGrace+time.Minute))
	expired, _ := u.Ensure(ctx, "svc", "prod")
	require.Empty(t, expired.PreviousKey)
}
```

Check `pkg/clock` for the test helper that freezes time (`grep -n "func " pkg/clock/*.go`) and use its real name in place of `clock.Set`; check `internal/secretbox` for the constructor's real signature (`grep -n "^func New" internal/secretbox/*.go`).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./app/queue/usecases/credentials/`
Expected: build failure.

- [ ] **Step 3: Write the credentials use case**

`app/queue/usecases/credentials/credentials.go`:

```go
package credentials

import (
	"context"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

type Store interface {
	Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error)
	SaveCredentials(ctx context.Context, c entities.Credentials) error
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
	Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error)
}

type Opened struct {
	Token       string
	SigningKey  string
	PreviousKey string
}

type CredentialsUsecase struct {
	store  Store
	sealer Sealer
}

func NewCredentialsUsecase(store Store, sealer Sealer) CredentialsUsecase {
	return CredentialsUsecase{store: store, sealer: sealer}
}

func (u CredentialsUsecase) Ensure(ctx context.Context, serviceID, environment string) (Opened, error) {
	c, found, err := u.store.Credentials(ctx, serviceID, environment)
	if err != nil {
		return Opened{}, err
	}
	if found {
		return u.Open(c)
	}
	return u.issue(ctx, entities.Credentials{ServiceID: serviceID, Environment: environment}, "")
}

func (u CredentialsUsecase) Rotate(ctx context.Context, serviceID, environment string) (Opened, error) {
	current, err := u.Ensure(ctx, serviceID, environment)
	if err != nil {
		return Opened{}, err
	}
	return u.issue(ctx, entities.Credentials{ServiceID: serviceID, Environment: environment}, current.SigningKey)
}

func (u CredentialsUsecase) Open(c entities.Credentials) (Opened, error) {
	token, err := u.sealer.Open(c.TokenCiphertext, c.TokenNonce, aad(c, "token"), c.KeyID)
	if err != nil {
		return Opened{}, err
	}
	key, err := u.sealer.Open(c.KeyCiphertext, c.KeyNonce, aad(c, "key"), c.KeyID)
	if err != nil {
		return Opened{}, err
	}
	previous, err := u.previous(c)
	return Opened{Token: string(token), SigningKey: string(key), PreviousKey: previous}, err
}

func (u CredentialsUsecase) previous(c entities.Credentials) (string, error) {
	if !c.PreviousKeyValid(clock.Now().UTC()) {
		return "", nil
	}
	raw, err := u.sealer.Open(c.PreviousKeyCiphertext, c.PreviousKeyNonce, aad(c, "previous"), c.KeyID)
	return string(raw), err
}

func aad(c entities.Credentials, part string) []byte {
	return []byte("queue|" + c.ServiceID + "|" + c.Environment + "|" + part)
}
```

`issue` and its helpers, in the same file:

```go
func (u CredentialsUsecase) issue(ctx context.Context, c entities.Credentials, previousKey string) (Opened, error) {
	token, tokenErr := entities.NewSecret()
	key, keyErr := entities.NewSecret()
	if err := errors.Join(tokenErr, keyErr); err != nil {
		return Opened{}, err
	}
	c.TokenHash = entities.HashToken(token)
	if err := u.sealAll(&c, token, key, previousKey); err != nil {
		return Opened{}, err
	}
	if err := u.store.SaveCredentials(ctx, c); err != nil {
		return Opened{}, err
	}
	return Opened{Token: token, SigningKey: key, PreviousKey: previousKey}, nil
}

func (u CredentialsUsecase) sealAll(c *entities.Credentials, token, key, previousKey string) error {
	var err error
	if c.TokenCiphertext, c.TokenNonce, c.KeyID, err = u.sealer.Seal([]byte(token), aad(*c, "token")); err != nil {
		return err
	}
	if c.KeyCiphertext, c.KeyNonce, _, err = u.sealer.Seal([]byte(key), aad(*c, "key")); err != nil {
		return err
	}
	if previousKey == "" {
		return nil
	}
	now := clock.Now().UTC()
	c.RotatedAt = &now
	c.PreviousKeyCiphertext, c.PreviousKeyNonce, _, err = u.sealer.Seal([]byte(previousKey), aad(*c, "previous"))
	return err
}
```

(add `"errors"` to the imports).

- [ ] **Step 4: Write the failing gateway tests**

`app/queue/usecases/gateway/gateway_test.go`:

```go
package gateway_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/queue/usecases/gateway"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type source struct {
	queues   []entities.Queue
	commands []entities.Command
	creds    []entities.Credentials
	done     []string
}

func (s *source) All(context.Context) ([]entities.Queue, error)               { return s.queues, nil }
func (s *source) PendingCommands(context.Context) ([]entities.Command, error) { return s.commands, nil }
func (s *source) AllCredentials(context.Context) ([]entities.Credentials, error) {
	return s.creds, nil
}
func (s *source) FinishCommand(_ context.Context, id string, _ time.Time) error {
	s.done = append(s.done, id)
	return nil
}

type services struct{}

func (services) FindServiceByID(_ context.Context, id string) (queues.ServiceRef, error) {
	if id == "s-worker" {
		return queues.ServiceRef{ID: id, Slug: "worker"}, nil
	}
	return queues.ServiceRef{}, apperror.NotFound("service not found")
}

type ports struct{}

func (ports) Port(context.Context, string, string) (int, error) { return 8081, nil }

type creds struct{}

func (creds) Ensure(_ context.Context, serviceID, _ string) (credentials.Opened, error) {
	return credentials.Opened{SigningKey: "key-" + serviceID}, nil
}

func placeImport(consumer string) entities.Queue {
	return entities.Queue{
		ID: "q-" + consumer, Name: "place-import", Environment: "prod", ConsumerID: consumer, Version: 3,
		Settings: entities.DefaultSettings("/hooks/place-import"), Publishers: []string{consumer, "s-importer", "s-new"},
	}
}

func Test_Desired_BuildsTargetsKeysAndPublisherHashes(t *testing.T) {
	src := &source{
		queues: []entities.Queue{placeImport("s-worker")},
		creds: []entities.Credentials{
			{ServiceID: "s-importer", Environment: "prod", TokenHash: "h-importer"},
			{ServiceID: "s-importer", Environment: "dev", TokenHash: "h-importer-dev"},
		},
		commands: []entities.Command{{ID: "c1", Kind: entities.CommandReplayAll}},
	}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Queues, 1)
	q := desired.Queues[0]
	require.Equal(t, "http://worker-prod:8081/hooks/place-import", q.Target)
	require.Equal(t, "key-s-worker", q.SigningKey)
	require.Equal(t, []string{"h-importer"}, q.PublisherHashes)
	require.Equal(t, "c1", desired.Commands[0].ID)
}

func Test_Desired_SkipsDeletedServices(t *testing.T) {
	src := &source{queues: []entities.Queue{placeImport("s-gone"), placeImport("s-worker")}}
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, gateway.NewStatsStore())

	desired, err := u.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired.Queues, 1)
	require.Equal(t, "q-s-worker", desired.Queues[0].ID)
}

func Test_Report_AndDone(t *testing.T) {
	src := &source{}
	stats := gateway.NewStatsStore()
	u := gateway.NewGatewayUsecase(src, services{}, ports{}, creds{}, stats)

	require.NoError(t, u.Report(ctx, []entities.Stats{{QueueID: "q1", Depth: 4}}))
	require.NoError(t, u.Done(ctx, "c9"))

	latest, ok := stats.Latest("q1")
	require.True(t, ok)
	require.EqualValues(t, 4, latest.Depth)
	require.False(t, latest.ReportedAt.IsZero())
	require.Equal(t, []string{"c9"}, src.done)
}
```

- [ ] **Step 5: Run them to see them fail**

Run: `go test ./app/queue/usecases/gateway/`
Expected: build failure.

- [ ] **Step 6: Write the stats store and the gateway use case**

`app/queue/usecases/gateway/statsstore.go`:

```go
package gateway

import (
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

type StatsStore struct {
	mu     sync.RWMutex
	latest map[string]entities.Stats
}

func NewStatsStore() *StatsStore {
	return &StatsStore{latest: map[string]entities.Stats{}}
}

func (s *StatsStore) Put(stats []entities.Stats, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range stats {
		st.ReportedAt = at
		s.latest[st.QueueID] = st
	}
}

func (s *StatsStore) Latest(queueID string) (entities.Stats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.latest[queueID]
	return st, ok
}
```

`app/queue/usecases/gateway/gateway.go`:

```go
package gateway

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

type QueueSource interface {
	All(ctx context.Context) ([]entities.Queue, error)
	PendingCommands(ctx context.Context) ([]entities.Command, error)
	FinishCommand(ctx context.Context, id string, at time.Time) error
	AllCredentials(ctx context.Context) ([]entities.Credentials, error)
}

type ServiceLookup interface {
	FindServiceByID(ctx context.Context, id string) (queues.ServiceRef, error)
}

type PortLookup interface {
	Port(ctx context.Context, serviceID, environment string) (int, error)
}

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type DesiredQueue struct {
	ID              string
	Name            string
	Environment     string
	Version         int
	Settings        entities.Settings
	Paused          bool
	Target          string
	PublisherHashes []string
	SigningKey      string
	PreviousKey     string
}

type Desired struct {
	Queues   []DesiredQueue
	Commands []entities.Command
}

type GatewayUsecase struct {
	queues   QueueSource
	services ServiceLookup
	ports    PortLookup
	creds    Credentials
	stats    *StatsStore
}

func NewGatewayUsecase(queues QueueSource, services ServiceLookup, ports PortLookup, creds Credentials, stats *StatsStore) GatewayUsecase {
	return GatewayUsecase{queues: queues, services: services, ports: ports, creds: creds, stats: stats}
}

func (u GatewayUsecase) Desired(ctx context.Context) (Desired, error) {
	all, err := u.queues.All(ctx)
	if err != nil {
		return Desired{}, err
	}
	hashes, err := u.tokenHashes(ctx)
	if err != nil {
		return Desired{}, err
	}
	result := make([]DesiredQueue, 0, len(all))
	for _, q := range all {
		d, ok, err := u.desiredQueue(ctx, q, hashes)
		if err != nil {
			return Desired{}, err
		}
		if ok {
			result = append(result, d)
		}
	}
	commands, err := u.queues.PendingCommands(ctx)
	return Desired{Queues: result, Commands: commands}, err
}

func (u GatewayUsecase) desiredQueue(ctx context.Context, q entities.Queue, hashes map[string]string) (DesiredQueue, bool, error) {
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if errors.Is(err, apperror.NotFound("")) {
		return DesiredQueue{}, false, nil
	}
	if err != nil {
		return DesiredQueue{}, false, err
	}
	target, err := u.target(ctx, consumer, q)
	if err != nil {
		return DesiredQueue{}, false, err
	}
	keys, err := u.creds.Ensure(ctx, consumer.ID, q.Environment)
	return DesiredQueue{
		ID: q.ID, Name: q.Name, Environment: q.Environment, Version: q.Version, Settings: q.Settings, Paused: q.Paused,
		Target: target, PublisherHashes: publisherHashes(q, hashes), SigningKey: keys.SigningKey, PreviousKey: keys.PreviousKey,
	}, true, err
}

func (u GatewayUsecase) target(ctx context.Context, consumer queues.ServiceRef, q entities.Queue) (string, error) {
	port, err := u.ports.Port(ctx, consumer.ID, q.Environment)
	return fmt.Sprintf("http://%s-%s:%d%s", consumer.Slug, q.Environment, port, q.Settings.Path), err
}

func (u GatewayUsecase) tokenHashes(ctx context.Context) (map[string]string, error) {
	all, err := u.queues.AllCredentials(ctx)
	hashes := make(map[string]string, len(all))
	for _, c := range all {
		hashes[c.ServiceID+"|"+c.Environment] = c.TokenHash
	}
	return hashes, err
}

func publisherHashes(q entities.Queue, hashes map[string]string) []string {
	result := []string{}
	for _, id := range q.Publishers {
		if h, ok := hashes[id+"|"+q.Environment]; ok && !slices.Contains(result, h) {
			result = append(result, h)
		}
	}
	return result
}

func (u GatewayUsecase) Report(_ context.Context, stats []entities.Stats) error {
	u.stats.Put(stats, clock.Now().UTC())
	return nil
}

func (u GatewayUsecase) Done(ctx context.Context, commandID string) error {
	return u.queues.FinishCommand(ctx, commandID, clock.Now().UTC())
}
```

In `Test_Desired_BuildsTargetsKeysAndPublisherHashes` the consumer `s-worker` has no token hash, so only `h-importer` is listed — matching the expectation.

- [ ] **Step 7: Run the tests**

Run: `go test ./app/queue/... && make lint`
Expected: PASS.

- [ ] **Step 8: Commit (rules layer)**

```bash
git add app/queue/usecases/credentials app/queue/usecases/gateway
git commit --amend --no-edit
```

Amend into Task 3's rules commit (it is still the branch tip and unpushed), keeping one rules commit; extend its message body with "credentials sealed per service and environment, rotated with a 10 minute grace; the gateway's view: targets, publisher token hashes and signing keys, stats kept in memory."

---

### Task 5: Handlers, injected config, wiring and integration tests

**Files:**
- Create: `app/queue/handlers/queues/queues.go`, `dto.go`
- Create: `app/queue/handlers/gatewayapi/gatewayapi.go`
- Create: `app/queue/repositories/envconfig/envconfig.go` (`PortLookup` over the config use case)
- Create: `app/deploy/repositories/queue/queue.go`
- Modify: `app/deploy/usecases/agents/agents.go`, `claim.go`
- Modify: `app/envconfig/entities/validate.go` (reserved names)
- Modify: `pkg/config/config.go` (`Queues`)
- Create: `cmd/controlplane/modules/queue.go`; Modify: `cmd/controlplane/options/options.go`
- Modify: `internal/integration/suite.go` (`Queues.Token`, `integration.QueuedToken`)
- Create: `internal/integration/tests/queues_test.go`
- Modify: `README.md` (configuration table: `PLATFORM_QUEUES_TOKEN`, `PLATFORM_QUEUES_GATEWAY_URL`; a "Queues" section)

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces (HTTP, used by plan 2 and plan 5):
  - Public: `GET /api/v1/queues?environment=&team=` → `{"queues":[QueueDTO]}`; `POST /api/v1/queues` (body `createDTO`) → 201 `QueueDTO`; `GET /api/v1/queues/:env/:name` → `QueueDTO`; `PATCH` (body `changeDTO`) → 200; `DELETE ?force=true` → 204; `POST .../pause`, `.../resume` → 200; `GET .../stats` → `{"stats":StatsDTO|null,"fresh":bool}`; `GET .../versions` → `{"versions":[...]}`; `GET .../dead` → `{"dead":[DeadLetter]}` (from the latest stats); `POST .../dead/:id/replay`, `POST .../dead/replay-all` → 202; `DELETE .../dead` → 202.
  - Gateway: `GET /queued/v1/desired` → `DesiredDTO`; `POST /queued/v1/stats` (body `{"stats":[Stats]}`) → 204; `POST /queued/v1/commands/:id/done` → 204.

DTOs (`app/queue/handlers/queues/dto.go`), durations as Go duration strings (`"30s"`) so the API reads like the UI:

```go
type backoffDTO struct {
	Kind    string `json:"kind"`
	Initial string `json:"initial"`
	Max     string `json:"max"`
}

type settingsDTO struct {
	Path            string     `json:"path"`
	RatePerSecond   float64    `json:"rate_per_second"`
	MaxInFlight     int        `json:"max_in_flight"`
	MaxAttempts     int        `json:"max_attempts"`
	Backoff         backoffDTO `json:"backoff"`
	DeliveryTimeout string     `json:"delivery_timeout"`
	Retention       string     `json:"retention"`
	MaxMessageBytes int        `json:"max_message_bytes"`
}

type QueueDTO struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Environment string      `json:"environment"`
	TeamID      string      `json:"team_id"`
	Consumer    string      `json:"consumer"`
	Publishers  []string    `json:"publishers"`
	Version     int         `json:"version"`
	Settings    settingsDTO `json:"settings"`
	Paused      bool        `json:"paused"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type createDTO struct {
	Name        string       `json:"name" binding:"required"`
	Environment string       `json:"environment" binding:"required"`
	Consumer    string       `json:"consumer" binding:"required"`
	Path        string       `json:"path" binding:"required"`
	Settings    *settingsDTO `json:"settings"`
	Publishers  []string     `json:"publishers"`
	Message     string       `json:"message"`
}

type changeDTO struct {
	Settings   *settingsDTO `json:"settings"`
	Publishers *[]string    `json:"publishers"`
	Message    string       `json:"message"`
}
```

`Consumer` and `Publishers` in `QueueDTO` are service **slugs**: the handler maps ids to slugs through a `ServiceNames` interface (`SlugOf(ctx, id) (string, error)`) backed by the catalog lookup; unknown ids are dropped. A settings DTO field that fails `time.ParseDuration` returns 400 with the field name.

The gateway desired DTO (`app/queue/handlers/gatewayapi/gatewayapi.go`) reuses the same `settingsDTO` shape (copy it into this package; the two packages must not import each other's handler types):

```go
type desiredQueueDTO struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Environment     string      `json:"environment"`
	Version         int         `json:"version"`
	Settings        settingsDTO `json:"settings"`
	Paused          bool        `json:"paused"`
	Target          string      `json:"target"`
	PublisherHashes []string    `json:"publisher_hashes"`
	SigningKey      string      `json:"signing_key"`
	PreviousKey     string      `json:"previous_key,omitempty"`
}

type commandDTO struct {
	ID       string `json:"id"`
	QueueID  string `json:"queue_id"`
	Kind     string `json:"kind"`
	Argument string `json:"argument,omitempty"`
}

type desiredDTO struct {
	Queues   []desiredQueueDTO `json:"queues"`
	Commands []commandDTO      `json:"commands"`
}
```

Injected config: `app/deploy/repositories/queue/queue.go`:

```go
package queue

import (
	"context"

	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
)

type Credentials interface {
	Ensure(ctx context.Context, serviceID, environment string) (credentials.Opened, error)
}

type Access struct {
	creds      Credentials
	gatewayURL string
}

func NewAccess(creds Credentials, gatewayURL string) Access {
	return Access{creds: creds, gatewayURL: gatewayURL}
}

func (a Access) QueueAccess(ctx context.Context, serviceID, environment string) (map[string]string, map[string]string, error) {
	opened, err := a.creds.Ensure(ctx, serviceID, environment)
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{"PLATFORM_QUEUE_URL": a.gatewayURL}
	secrets := map[string]string{"PLATFORM_QUEUE_TOKEN": opened.Token, "PLATFORM_QUEUE_SIGNING_KEY": opened.SigningKey}
	return env, secrets, nil
}
```

In `app/deploy/usecases/agents/agents.go` add the consumer interface and field:

```go
type QueueAccess interface {
	QueueAccess(ctx context.Context, serviceID, environment string) (env, secrets map[string]string, err error)
}
```

`NewAgentsUsecase(cfg, store, services, configs, queues QueueAccess, publisher)`; in `claim.go` `workload` merges the maps into `resolved.Env` and `resolved.Secrets` after `ResolveConfig` succeeds (a helper `withQueueAccess(ctx, resolved, serviceID, env) (entities.ResolvedConfig, error)`; it allocates the maps when nil). Update the agents unit-test fakes with a `QueueAccess` fake returning fixed values and add `Test_Claim_AddsQueueAccessToTheWorkload` asserting the three names appear.

Reserved names in `app/envconfig/entities/validate.go`:

```go
var reserved = map[string]bool{"PLATFORM_QUEUE_URL": true, "PLATFORM_QUEUE_TOKEN": true, "PLATFORM_QUEUE_SIGNING_KEY": true}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid(fmt.Sprintf("%q is not a valid name: use UPPER_SNAKE_CASE", name))
	}
	if reserved[name] {
		return apperror.Invalid(fmt.Sprintf("%s is set by the platform", name))
	}
	return nil
}
```

and add a case to `Test_Validate_Rules`: `"reserved name": func(r *entities.Revision) { r.Env["PLATFORM_QUEUE_TOKEN"] = "x" }`.

Config (`pkg/config/config.go`):

```go
type Queues struct {
	GatewayURL string `mapstructure:"gateway_url" default:"http://queued:8080"`
	Token      string `mapstructure:"token"`
}
```

with field `Queues Queues \`mapstructure:"queues"\`` on `Config` (env `PLATFORM_QUEUES_GATEWAY_URL`, `PLATFORM_QUEUES_TOKEN`).

Gateway handler authentication mirrors `agentapi.authenticate`: empty token → `apperror.New(apperror.CodeNotConfigured, "the queue gateway is not configured on this control plane")`; wrong bearer → `apperror.New(apperror.CodeUnauthenticated, "invalid queue gateway token")`. Routes: `public.Group("/queued/v1", h.authenticate)`.

Wiring `cmd/controlplane/modules/queue.go` follows `modules/envconfig.go`: provide `mysql.NewQueueRepository`, `catalog.NewLookup`, the envconfig port adapter, `credentials.NewCredentialsUsecase`, `gateway.NewStatsStore`, `gateway.NewGatewayUsecase`, `queues.NewQueuesUsecase`, both handlers; one `fx.Provide` block of interface casts per consumer, functions ≤ 20 lines each (split into `queueAdapterInjections` and `queueUsecaseInjections` like `modules/deploy.go`); invoke `RegisterQueuesRoutes` and `RegisterGatewayRoutes`. In `modules/deploy.go` add `func(u credentials.CredentialsUsecase) deployQueue.Credentials { return u }`, provide `deployQueue.NewAccess` through a function taking `config.Config` (`func(c deployQueue.Credentials, cfg config.Config) deployQueue.Access { return deployQueue.NewAccess(c, cfg.Queues.GatewayURL) }`) and cast `deployQueue.Access` to `agents.QueueAccess`. Register `modules.Queue()` in `options.Options()`.

- [ ] **Step 1: Write the failing integration tests**

`internal/integration/tests/queues_test.go`:

```go
package tests

import (
	"net/http"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

type queueDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Consumer   string   `json:"consumer"`
	Publishers []string `json:"publishers"`
	Version    int      `json:"version"`
	Paused     bool     `json:"paused"`
	Settings   struct {
		MaxAttempts     int    `json:"max_attempts"`
		DeliveryTimeout string `json:"delivery_timeout"`
	} `json:"settings"`
}

func setupQueue(t *testing.T) (*integration.Suite, people) {
	t.Helper()
	s, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "places-worker", "name": "Places worker", "owner_team": "places", "image_repository": "docker.io/adrianozdp/places-api",
	}, http.StatusCreated, nil)
	return s, p
}

func createQueue(t *testing.T, c *integration.Client, env string, status int) queueDTO {
	t.Helper()
	var q queueDTO
	c.MustJSON(t, http.MethodPost, "/api/v1/queues", map[string]any{
		"name": "place-import", "environment": env, "consumer": "places-worker", "path": "/hooks/place-import",
		"publishers": []string{"places-api"},
	}, status, &q)
	return q
}

func Test_QueuesAreCreatedWithDefaultsAndEditedByPolicy(t *testing.T) {
	_, p := setupQueue(t)

	created := createQueue(t, p.bob, "dev", http.StatusCreated)
	bobProd := p.bob.Do(t, http.MethodPost, "/api/v1/queues", map[string]any{"name": "place-import", "environment": "prod", "consumer": "places-worker", "path": "/hooks/place-import"})
	updated := p.bob.Do(t, http.MethodPatch, "/api/v1/queues/dev/place-import", map[string]any{
		"settings": map[string]any{"path": "/hooks/place-import", "rate_per_second": 5, "max_in_flight": 2, "max_attempts": 9,
			"backoff": map[string]string{"kind": "fixed", "initial": "2s", "max": "2s"}, "delivery_timeout": "10s",
			"retention": "24h", "max_message_bytes": 65536},
		"message": "slower",
	})
	eve := p.eve.Do(t, http.MethodPatch, "/api/v1/queues/dev/place-import", map[string]any{"message": "x"})

	require.Equal(t, "places-worker", created.Consumer)
	require.ElementsMatch(t, []string{"places-worker", "places-api"}, created.Publishers)
	require.Equal(t, 5, created.Settings.MaxAttempts)
	require.Equal(t, "30s", created.Settings.DeliveryTimeout)
	require.Equal(t, http.StatusForbidden, bobProd.Status, bobProd.Body)
	require.Equal(t, http.StatusOK, updated.Status, updated.Body)
	require.Contains(t, updated.Body, `"version":2`)
	require.Equal(t, http.StatusForbidden, eve.Status, eve.Body)
}

func Test_TheGatewaySeesDesiredQueuesAndReportsStats(t *testing.T) {
	s, p := setupQueue(t)
	empty := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)

	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	reported := s.Queued(t, http.MethodPost, "/queued/v1/stats", map[string]any{"stats": []map[string]any{{"queue_id": q.ID, "depth": 7}}})
	anonymous := s.Raw(t, http.MethodGet, "/queued/v1/desired", nil, nil)
	var stats struct {
		Stats struct{ Depth int64 } `json:"stats"`
		Fresh bool                  `json:"fresh"`
	}
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/queues/dev/place-import/stats", nil, http.StatusOK, &stats)

	require.Equal(t, http.StatusOK, empty.Status, empty.Body)
	require.Contains(t, empty.Body, `"queues":[]`)
	require.Contains(t, desired.Body, `"target":"http://places-worker-dev:8080/hooks/place-import"`)
	require.Contains(t, desired.Body, `"signing_key":"`)
	require.Equal(t, http.StatusNoContent, reported.Status, reported.Body)
	require.Equal(t, http.StatusUnauthorized, anonymous.Status)
	require.EqualValues(t, 7, stats.Stats.Depth)
	require.True(t, stats.Fresh)
}

func Test_DeleteNeedsAnEmptyQueueOrForce(t *testing.T) {
	s, p := setupQueue(t)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)
	s.Queued(t, http.MethodPost, "/queued/v1/stats", map[string]any{"stats": []map[string]any{{"queue_id": q.ID, "depth": 2}}})

	held := p.bob.Do(t, http.MethodDelete, "/api/v1/queues/dev/place-import", nil)
	forced := p.bob.Do(t, http.MethodDelete, "/api/v1/queues/dev/place-import?force=true", nil)
	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)

	require.Equal(t, http.StatusConflict, held.Status, held.Body)
	require.Equal(t, http.StatusNoContent, forced.Status, forced.Body)
	require.Contains(t, desired.Body, `"kind":"delete"`)
}

func Test_DeadLetterCommandsReachTheGateway(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)

	replay := p.bob.Do(t, http.MethodPost, "/api/v1/queues/dev/place-import/dead/m1/replay", nil)
	desired := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)
	var parsed struct {
		Commands []struct{ ID, Kind, Argument string } `json:"commands"`
	}
	decode(t, desired.Body, &parsed)
	done := s.Queued(t, http.MethodPost, "/queued/v1/commands/"+parsed.Commands[0].ID+"/done", nil)
	after := s.Queued(t, http.MethodGet, "/queued/v1/desired", nil)

	require.Equal(t, http.StatusAccepted, replay.Status, replay.Body)
	require.Equal(t, "replay", parsed.Commands[0].Kind)
	require.Equal(t, "m1", parsed.Commands[0].Argument)
	require.Equal(t, http.StatusNoContent, done.Status)
	require.Contains(t, after.Body, `"commands":[]`)
}

func Test_ReservedQueueNamesCannotBeSetByHand(t *testing.T) {
	_, p := setupQueue(t)

	resp := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-worker/dev/revisions", map[string]any{"set_env": map[string]string{"PLATFORM_QUEUE_TOKEN": "x"}})

	require.Equal(t, http.StatusBadRequest, resp.Status, resp.Body)
	require.Contains(t, resp.Body, "set by the platform")
}
```

Add to `internal/integration`: `const QueuedToken = "queued-test-token"`; set `cfg.Queues.Token = QueuedToken` in `testConfig`; a client helper next to `Agent`:

```go
func (s *Suite) Queued(t *testing.T, method, path string, body any) Response {
	t.Helper()
	return s.Raw(t, method, path, encodeBytes(t, body), map[string]string{
		"Authorization": "Bearer " + QueuedToken, "Content-Type": "application/json",
	})
}
```

(reuse the encoder the `Agent` helper uses — read `internal/integration/client.go:131` and follow it exactly). Add a `decode(t, body, out)` helper to `helpers_test.go` if none exists (`require.NoError(t, json.Unmarshal([]byte(body), out))`).

Extend the deploy integration test that claims a workload (`Test_ADeployIsClaimedRunAndBecomesLive` in `internal/integration/tests/deploys_test.go`) with assertions that the claimed workload's `env` holds `PLATFORM_QUEUE_URL` and its `secrets` hold `PLATFORM_QUEUE_TOKEN` and `PLATFORM_QUEUE_SIGNING_KEY`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test -count=1 ./internal/integration/...`
Expected: FAIL (404 on `/api/v1/queues`, `s.Queued` undefined).

- [ ] **Step 3: Implement the handlers, adapters, config, reserved names, agents change and wiring** as specified in this task's Interfaces block. Keep each handler method to: bind → call use case → `httperror.Abort` or JSON. Settings conversion lives in `dto.go` (`toSettings(settingsDTO) (entities.Settings, error)`, `fromSettings(entities.Settings) settingsDTO`), each ≤ 20 lines; split duration parsing into `parseDurations(...)` if needed.

- [ ] **Step 4: Run everything**

Run: `make lint && make test`
Expected: PASS, including the new integration tests and the extended deploy test.

- [ ] **Step 5: Verify each layer commit builds**

Run: `git rebase -q --exec "go build ./... && go vet ./..." main`
Expected: three commits, each builds.

- [ ] **Step 6: Commit (handlers layer)**

```bash
git add -A
git commit -m "Serve queues over the API and to the gateway, and hand services their queue access"
```

Body: the public routes and policy, the gateway routes and token, the three injected names and that they are reserved, README updates.

- [ ] **Step 7: Ship**

Push the branch, open the PR (title "Managed queues: control plane", body: what exists after this plan, that no gateway runs yet so queues are definitions only, the new env vars, migration 000004), merge with rebase after CI, run the `Release` workflow, then set `PLATFORM_QUEUES_TOKEN` in infra (that lands with plan 3; until then the gateway API answers 503, which is the designed behaviour).

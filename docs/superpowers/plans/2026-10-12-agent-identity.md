# Agent identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An agent calls `/api/v1` with `Authorization: Bearer plat_…`, a token that belongs to a human, can do at most what its owner, its scopes and its teams all allow, is told `err_approval_required` on protected environments unless an admin granted bypass, and shows up in the audit log as "owner via token".

**Architecture:** Tokens live in the identity domain (`access_tokens` table, `app/identity/usecases/tokens`). `actor.Actor` gains a `Token` value; `authz.Policy.Can` checks the owner exactly as today, then the token's scope and teams, then the protected-environment rule, and publishes an audit event when bypass is used. The API guard accepts a Bearer header (no CSRF, session cookie ignored, `read` scope required on safe methods); page guards refuse it. Reads are limited to the token's teams through new `…For(ctx, actor, …)` usecase methods that the JSON handlers call; the web UI keeps calling the existing methods because it never carries a token.

**Tech Stack:** Go 1.26, Gin, Uber fx, GORM on MySQL 8, golang-migrate, testify, htmx templates; integration tests in `internal/integration`.

**Spec:** `docs/superpowers/specs/2026-10-12-agent-identity-design.md` (all sections). Sub-projects 2–7 are out of scope.

## Global Constraints

- Function bodies ≤ 20 lines, at most 2 levels of nesting, **no comments** in any file: Go, SQL, HTML, tests (user rule; a hook enforces it).
- One commit per layer, bottom-up: storage (Task 1), rules (Tasks 2–4), handlers + wiring + UI + integration tests (Tasks 5–7), docs (Task 8). No fix/wip commits, no `Co-Authored-By`, do not push.
- Token format: `plat_` + 32 random bytes in base62, left-padded to 43 characters. Only the SHA-256 hex hash is stored. Display prefix: the first 8 characters after `plat_`.
- Token name: `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, unique per owner among tokens that are not revoked: a stored generated column `active_name` (the name while `revoked_at IS NULL`, else NULL) carries the unique key `(owner_id, active_name)`. Revoking frees the name; an expired but unrevoked token still holds it.
- Expiry: required, `expires_in_days` 1–365, 0 or missing means 90.
- Teams: explicit, non-empty list of team slugs in the API, stored as team ids.
- Scopes: `read`, `logs`, `services`, `services:admin`, `builds`, `config`, `secrets`, `deploy`, `protected`, `teams:admin`, `tokens:admin`. At least one. `logs` (reading logs) is a valid scope in this sub-project only: no endpoint uses it yet, `read` does not imply it, and it is listed in the UI and in whoami.
- Action → scope: `service.create`, `service.edit`, `queue.edit` → `services`; `service.delete`, `service.transfer` → `services:admin`; `release.create` → `builds`; `config.edit` → `config`; `secrets.edit` → `secrets`; `deploy` → `deploy`; `team.members`, `teams.manage` → `teams:admin`; `tokens.manage` → `tokens:admin`; `admins.manage` → never for a token.
- Protected rule applies to `config.edit`, `secrets.edit`, `queue.edit` and `deploy` on environments whose `protected` flag is set: no `protected` scope → 403 `err_missing_scope`; `protected` without bypass → 403 `err_approval_required`; both → allowed and an audit event `token.bypass_used`.
- Bypass is granted only by a platform admin in a browser session. A token never issues bypass, even one that has it.
- A token with `tokens:admin` creates tokens for its own owner whose scopes and teams are subsets of its own; a child that would outlive its parent silently gets the parent's expiry.
- Error codes: 401 `err_token_invalid`, `err_token_expired`, `err_token_revoked`; 403 `err_missing_scope` (with `required_scope`), `err_team_not_granted` (with `team`, the team id), `err_approval_required`; owner lacking the role stays 403 `err_forbidden`. The body keeps today's `{"error": {...}}` wrapper; sub-project 3 owns the response shape.
- Reads limited by token teams: services, config, releases, deployments, queues, audit. Other teams' resources are left out of lists and answer 404. Teams, members, users and environments stay readable with `read`.
- `GET` on `/api/v1` with a token needs `read`, except `/api/v1/whoami`, `/api/v1/me` and `/api/v1/tokens` (the last needs `tokens:admin` from a token).
- `last_used_at`/`last_used_ip` are written at most once a minute per token unless the IP changes.

## Review Focus

1. The owner loses a role after the token was made: the very next request must answer `err_forbidden`. Test in Task 6 (`Test_LosingTheRoleTakesItFromTheToken`).
2. Malformed `Authorization` headers: `Bearer` with garbage, `Bearer` with a session token, lowercase `bearer`, and a non-Bearer scheme next to a valid cookie. Garbage → 401 `err_token_invalid`; a non-Bearer scheme falls back to the cookie. Test in Task 5 (`Test_MalformedAuthorizationHeaders`).
3. The secret must never come back after creation: list, whoami and the tokens page show only the prefix. Test in Task 6 (`Test_WhoamiAndTheSecretIsShownOnce`) and Task 7 (`Test_TheTokensPageShowsTheSecretOnce`).
4. `?team=<team the token lacks>` on list endpoints must return an empty list, not that team's data. Test in Task 6 (`Test_ReadsOutsideTheTokensTeamsAreHidden`).
5. An agent polling every second must not write the tokens row every second. Test in Task 3 (`Test_Authenticate_TouchesAtMostOncePerMinute`).

## File Structure

| File | Responsibility |
|---|---|
| `migrations/000005_access_tokens.up.sql` / `.down.sql` | `access_tokens`, audit token columns |
| `internal/testdb/testdb.go` | add `access_tokens` to `tables` |
| `pkg/scope/scope.go` | `Scope`, the eleven scopes, `Parse` |
| `pkg/apperror/apperror.go` | new codes, `RequiredScope` and `Team` fields, constructors |
| `pkg/actor/actor.go` | `Token`, `ViaToken`, `Sees`, `SessionAdmin` |
| `pkg/events/events.go` | `TokenID`, `TokenName`, `TargetToken` |
| `app/identity/entities/access_token.go` | `AccessToken`, secret generation, `Check`, errors, `TokenAuthenticated` |
| `app/identity/entities/token_draft.go` | `TokenDraft`, `TokenGrant`, `IssuedToken`, `Whoami`, validation |
| `app/identity/repositories/mysql/token_model.go`, `tokens.go` | storage |
| `app/identity/repositories/teams/teams.go` | team slug ↔ id adapter |
| `app/identity/usecases/tokens/tokens.go`, `grant.go`, `authenticate.go` | create, list, revoke, whoami, authenticate |
| `app/authz/authz.go`, `app/authz/token.go` | owner rules, token rules, bypass event, `CanRead` |
| `app/audit/entities/entities.go`, `repositories/mysql/*`, `usecases/record`, `usecases/query/visible.go`, `repositories/lookups/lookups.go` | token on audit rows, filter by token and teams |
| `app/catalog/usecases/services/visible.go` | `GetFor`, `ListFor` |
| `app/envconfig/usecases/configs/visible.go` | `CurrentFor`, `HistoryFor`, `SecretsFor` |
| `app/deploy/usecases/deployments/visible.go` | `HistoryFor`, `GetFor` |
| `app/release/usecases/releases/visible.go` | `ListFor`, `GetFor` |
| `app/queue/usecases/queues/visible.go` | `GetFor`, `ListFor`, `VersionsFor`, `StatsFor` |
| 9 event publishers (listed in Task 2) | copy the token onto events |
| `app/identity/handlers/guard/guard.go`, `bearer.go` | Bearer authentication |
| `app/identity/handlers/tokens/tokens.go`, `dto.go`, `whoami.go` | `/api/v1/tokens`, `/api/v1/whoami` |
| `internal/httperror/httperror.go` | statuses for the new codes |
| API handlers in catalog, envconfig, deploy, release, queue, audit | call the `…For` methods |
| `app/web/handlers/tokens/tokens.go`, `templates/pages/tokens.html`, `templates/layout.html` | Access tokens page |
| `app/web/activity.go`, `templates/partials/components.html`, `handlers/audit/audit.go`, `templates/pages/audit.html` | "via token" and token filter |
| `cmd/controlplane/modules/identity.go`, `authz.go`, `web.go` | wiring |
| `internal/integration/client.go` | Bearer clients |
| `internal/integration/tests/tokens_test.go`, `token_access_test.go`, `token_pages_test.go` | end-to-end |
| `README.md` | Access tokens section |

---

### Task 1: Storage — migration, scopes, token entity and repository, audit columns

**Files:**
- Create: `migrations/000005_access_tokens.up.sql`, `migrations/000005_access_tokens.down.sql`
- Modify: `internal/testdb/testdb.go` (`tables`)
- Create: `pkg/scope/scope.go`, `pkg/scope/scope_test.go`
- Modify: `pkg/apperror/apperror.go` (token codes only)
- Create: `app/identity/entities/access_token.go`, `app/identity/entities/access_token_test.go`
- Create: `app/identity/repositories/mysql/token_model.go`, `app/identity/repositories/mysql/tokens.go`, `app/identity/repositories/mysql/tokens_test.go`
- Modify: `app/audit/entities/entities.go`, `app/audit/repositories/mysql/model.go`, `app/audit/repositories/mysql/events.go`, `app/audit/repositories/mysql/events_test.go`

**Interfaces:**
- Produces:

```go
package scope
type Scope string
const Read, Logs, Services, ServicesAdmin, Builds, Config, Secrets, Deploy, Protected, TeamsAdmin, TokensAdmin Scope
var All []Scope
func Parse(names []string) ([]Scope, error)

package apperror
const CodeTokenInvalid, CodeTokenExpired, CodeTokenRevoked Code

package entities
type AccessToken struct {
	ID, OwnerID, Name, Hash, Prefix string
	Scopes           []scope.Scope
	TeamIDs          []string
	Bypass           bool
	CreatedAt        time.Time
	CreatedByTokenID string
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	LastUsedIP       string
	RevokedAt        *time.Time
}
type TokenSecret struct{ Raw, Prefix, Hash string }
func NewTokenSecret() (TokenSecret, error)
func LooksLikeToken(raw string) bool
var ErrTokenInvalid, ErrTokenExpired, ErrTokenRevoked *apperror.AppError

package mysql
func NewTokenRepository(db *gorm.DB) TokenRepository
func (r TokenRepository) Create(ctx context.Context, t entities.AccessToken) error
func (r TokenRepository) FindByHash(ctx context.Context, hash string) (entities.AccessToken, error)
func (r TokenRepository) FindByID(ctx context.Context, id string) (entities.AccessToken, error)
func (r TokenRepository) ListByOwner(ctx context.Context, ownerID string) ([]entities.AccessToken, error)
func (r TokenRepository) ListAll(ctx context.Context) ([]entities.AccessToken, error)
func (r TokenRepository) Revoke(ctx context.Context, id string, at time.Time) error
func (r TokenRepository) Touch(ctx context.Context, id string, at time.Time, ip string) error

package audit entities
type Event struct { ...; TokenID, TokenName string }
type Filter struct { ...; TokenID string; TeamIDs []string }
type Query struct { ...; Token string }
```

- [ ] **Step 1: Write the migration**

`migrations/000005_access_tokens.up.sql`:

```sql
CREATE TABLE access_tokens (
    id CHAR(36) NOT NULL PRIMARY KEY,
    owner_id CHAR(36) NOT NULL,
    name VARCHAR(64) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    display_prefix CHAR(8) NOT NULL,
    scopes JSON NOT NULL,
    team_ids JSON NOT NULL,
    bypass BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    created_by_token_id CHAR(36) NOT NULL DEFAULT '',
    expires_at DATETIME(3) NOT NULL,
    last_used_at DATETIME(3) NULL,
    last_used_ip VARCHAR(45) NOT NULL DEFAULT '',
    revoked_at DATETIME(3) NULL,
    active_name VARCHAR(64) AS (IF(revoked_at IS NULL, name, NULL)) STORED,
    UNIQUE KEY access_tokens_name (owner_id, active_name),
    UNIQUE KEY access_tokens_hash (token_hash),
    KEY access_tokens_owner (owner_id)
);

ALTER TABLE audit_events
    ADD COLUMN token_id CHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN token_name VARCHAR(64) NOT NULL DEFAULT '',
    ADD KEY audit_events_token (token_id);
```

`migrations/000005_access_tokens.down.sql`:

```sql
ALTER TABLE audit_events
    DROP KEY audit_events_token,
    DROP COLUMN token_name,
    DROP COLUMN token_id;

DROP TABLE access_tokens;
```

In `internal/testdb/testdb.go` append `"access_tokens"` to `tables`.

- [ ] **Step 2: Write the failing tests for scopes and the secret**

`pkg/scope/scope_test.go`:

```go
package scope_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func Test_Parse_KeepsKnownScopesOnceInOrder(t *testing.T) {
	got, err := scope.Parse([]string{"deploy", "read", "deploy"})

	require.NoError(t, err)
	require.Equal(t, []scope.Scope{scope.Deploy, scope.Read}, got)
}

func Test_Parse_KnowsTheLogsScope(t *testing.T) {
	got, err := scope.Parse([]string{"logs"})

	require.NoError(t, err)
	require.Equal(t, []scope.Scope{scope.Logs}, got)
}

func Test_Parse_RejectsUnknownScopes(t *testing.T) {
	_, err := scope.Parse([]string{"read", "root"})

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Contains(t, err.Error(), `"root"`)
}
```

`app/identity/entities/access_token_test.go`:

```go
package entities_test

import (
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/stretchr/testify/require"
)

func Test_NewTokenSecret_IsPrefixedBase62AndHashed(t *testing.T) {
	secret, err := entities.NewTokenSecret()

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(secret.Raw, "plat_"))
	require.Len(t, secret.Raw, len("plat_")+43)
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, secret.Raw)
	require.Equal(t, secret.Raw[5:13], secret.Prefix)
	require.Equal(t, entities.HashToken(secret.Raw), secret.Hash)
	require.True(t, entities.LooksLikeToken(secret.Raw))
}

func Test_LooksLikeToken_RejectsOtherStrings(t *testing.T) {
	for _, raw := range []string{"", "plat_", "plat_short", strings.Repeat("a", 64), "PLAT_" + strings.Repeat("a", 43)} {
		require.False(t, entities.LooksLikeToken(raw), raw)
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./pkg/scope/ ./app/identity/entities/`
Expected: FAIL, `package github.com/klzn-labs/kalzone/pkg/scope is not in std` / `undefined: entities.NewTokenSecret`.

- [ ] **Step 4: Implement scopes, token codes and the entity**

`pkg/scope/scope.go`:

```go
package scope

import (
	"fmt"
	"slices"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type Scope string

const (
	Read          Scope = "read"
	Logs          Scope = "logs"
	Services      Scope = "services"
	ServicesAdmin Scope = "services:admin"
	Builds        Scope = "builds"
	Config        Scope = "config"
	Secrets       Scope = "secrets"
	Deploy        Scope = "deploy"
	Protected     Scope = "protected"
	TeamsAdmin    Scope = "teams:admin"
	TokensAdmin   Scope = "tokens:admin"
)

var All = []Scope{Read, Logs, Services, ServicesAdmin, Builds, Config, Secrets, Deploy, Protected, TeamsAdmin, TokensAdmin}

func Parse(names []string) ([]Scope, error) {
	result := make([]Scope, 0, len(names))
	for _, name := range names {
		s := Scope(name)
		if !slices.Contains(All, s) {
			return nil, apperror.Invalid(fmt.Sprintf("unknown scope %q", name))
		}
		if !slices.Contains(result, s) {
			result = append(result, s)
		}
	}
	return result, nil
}
```

In `pkg/apperror/apperror.go` add to the `const` block:

```go
	CodeTokenInvalid     Code = "err_token_invalid"
	CodeTokenExpired     Code = "err_token_expired"
	CodeTokenRevoked     Code = "err_token_revoked"
```

`app/identity/entities/access_token.go`:

```go
package entities

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

const (
	TokenPrefix     = "plat_"
	tokenBodyLength = 43
	displayLength   = 8
)

var (
	ErrTokenInvalid = apperror.New(apperror.CodeTokenInvalid, "this access token is not valid")
	ErrTokenExpired = apperror.New(apperror.CodeTokenExpired, "this access token has expired, create a new one")
	ErrTokenRevoked = apperror.New(apperror.CodeTokenRevoked, "this access token was revoked")
)

type AccessToken struct {
	ID               string
	OwnerID          string
	Name             string
	Hash             string
	Prefix           string
	Scopes           []scope.Scope
	TeamIDs          []string
	Bypass           bool
	CreatedAt        time.Time
	CreatedByTokenID string
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	LastUsedIP       string
	RevokedAt        *time.Time
}

type TokenSecret struct {
	Raw    string
	Prefix string
	Hash   string
}

func NewTokenSecret() (TokenSecret, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return TokenSecret{}, err
	}
	body := new(big.Int).SetBytes(random).Text(62)
	body = strings.Repeat("0", tokenBodyLength-len(body)) + body
	raw := TokenPrefix + body
	return TokenSecret{Raw: raw, Prefix: body[:displayLength], Hash: HashToken(raw)}, nil
}

func LooksLikeToken(raw string) bool {
	return strings.HasPrefix(raw, TokenPrefix) && len(raw) == len(TokenPrefix)+tokenBodyLength
}
```

- [ ] **Step 5: Run them to verify they pass**

Run: `go test ./pkg/scope/ ./app/identity/entities/`
Expected: PASS.

- [ ] **Step 6: Write the failing repository tests**

`app/identity/repositories/mysql/tokens_test.go`:

```go
package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

var created = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func token(id, owner, name string) entities.AccessToken {
	return entities.AccessToken{
		ID: id, OwnerID: owner, Name: name, Hash: "hash-" + id, Prefix: "abcd1234",
		Scopes: []scope.Scope{scope.Read, scope.Deploy}, TeamIDs: []string{"t1", "t2"},
		CreatedAt: created, ExpiresAt: created.Add(90 * 24 * time.Hour),
	}
}

func Test_Tokens_RoundTripAndLookups(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "claude-local")))
	require.NoError(t, repo.Create(ctx, token("k2", "u2", "claude-local")))

	byHash, err := repo.FindByHash(ctx, "hash-k1")
	require.NoError(t, err)
	mine, _ := repo.ListByOwner(ctx, "u1")
	all, _ := repo.ListAll(ctx)
	_, missing := repo.FindByHash(ctx, "nope")
	_, unknown := repo.FindByID(ctx, "nope")

	require.Equal(t, token("k1", "u1", "claude-local"), byHash)
	require.Len(t, mine, 1)
	require.Len(t, all, 2)
	require.ErrorIs(t, missing, entities.ErrTokenInvalid)
	require.ErrorIs(t, unknown, apperror.NotFound(""))
}

func Test_Tokens_NamesAreUniquePerOwnerWhileNotRevoked(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	expired := token("k1", "u1", "ci")
	expired.ExpiresAt = created.Add(-time.Hour)
	require.NoError(t, repo.Create(ctx, expired))

	taken := repo.Create(ctx, token("k2", "u1", "ci"))
	otherOwner := repo.Create(ctx, token("k3", "u2", "ci"))

	require.ErrorIs(t, taken, apperror.Conflict(""))
	require.NoError(t, otherOwner)
}

func Test_Tokens_ARevokedTokenFreesItsName(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "ci")))
	require.NoError(t, repo.Revoke(ctx, "k1", created))
	require.NoError(t, repo.Create(ctx, token("k2", "u1", "ci")))
	require.NoError(t, repo.Revoke(ctx, "k2", created))

	err := repo.Create(ctx, token("k3", "u1", "ci"))
	mine, _ := repo.ListByOwner(ctx, "u1")

	require.NoError(t, err)
	require.Len(t, mine, 3)
}

func Test_Tokens_RevokeAndTouch(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "ci")))
	later := created.Add(time.Hour)

	require.NoError(t, repo.Touch(ctx, "k1", later, "10.0.0.7"))
	require.NoError(t, repo.Revoke(ctx, "k1", later))
	require.NoError(t, repo.Revoke(ctx, "k1", later.Add(time.Hour)))
	got, err := repo.FindByID(ctx, "k1")

	require.NoError(t, err)
	require.Equal(t, later, got.LastUsedAt.UTC())
	require.Equal(t, "10.0.0.7", got.LastUsedIP)
	require.Equal(t, later, got.RevokedAt.UTC())
}
```

Append to `app/audit/repositories/mysql/events_test.go`:

```go
func Test_List_FiltersByTokenAndByTeams(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	viaToken := event("e1", 1, "t1", "s1", "dev")
	viaToken.TokenID, viaToken.TokenName = "k1", "claude-local"
	require.NoError(t, repo.Insert(ctx, viaToken))
	require.NoError(t, repo.Insert(ctx, event("e2", 2, "t2", "s2", "dev")))
	require.NoError(t, repo.Insert(ctx, event("e3", 3, "", "u1", "")))

	byToken, err := repo.List(ctx, entities.Filter{TokenID: "k1", Limit: 10})
	require.NoError(t, err)
	byTeams, _ := repo.List(ctx, entities.Filter{TeamIDs: []string{"t1", "t2"}, Limit: 10})

	require.Equal(t, []string{"e1"}, ids(byToken))
	require.Equal(t, "claude-local", byToken[0].TokenName)
	require.Equal(t, []string{"e2", "e1"}, ids(byTeams))
}
```

- [ ] **Step 7: Run them to verify they fail**

Run: `go test -p 1 ./app/identity/repositories/mysql/ ./app/audit/repositories/mysql/`
Expected: FAIL, `undefined: mysql.NewTokenRepository`, `unknown field TokenID in struct literal`.

- [ ] **Step 8: Implement the token repository**

`app/identity/repositories/mysql/token_model.go`:

```go
package mysql

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
)

type accessToken struct {
	ID               string `gorm:"primaryKey"`
	OwnerID          string
	Name             string
	TokenHash        string
	DisplayPrefix    string
	Scopes           []byte
	TeamIDs          []byte `gorm:"column:team_ids"`
	Bypass           bool
	CreatedAt        time.Time
	CreatedByTokenID string `gorm:"column:created_by_token_id"`
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	LastUsedIP       string `gorm:"column:last_used_ip"`
	RevokedAt        *time.Time
}

func (accessToken) TableName() string { return "access_tokens" }

func fromAccessToken(t entities.AccessToken) (accessToken, error) {
	scopes, err := json.Marshal(t.Scopes)
	if err != nil {
		return accessToken{}, err
	}
	teams, err := json.Marshal(t.TeamIDs)
	return accessToken{
		ID: t.ID, OwnerID: t.OwnerID, Name: t.Name, TokenHash: t.Hash, DisplayPrefix: t.Prefix,
		Scopes: scopes, TeamIDs: teams, Bypass: t.Bypass, CreatedAt: t.CreatedAt,
		CreatedByTokenID: t.CreatedByTokenID, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt,
		LastUsedIP: t.LastUsedIP, RevokedAt: t.RevokedAt,
	}, err
}

func (m accessToken) toEntity() (entities.AccessToken, error) {
	t := entities.AccessToken{
		ID: m.ID, OwnerID: m.OwnerID, Name: m.Name, Hash: m.TokenHash, Prefix: m.DisplayPrefix,
		Bypass: m.Bypass, CreatedAt: m.CreatedAt.UTC(), CreatedByTokenID: m.CreatedByTokenID,
		ExpiresAt: m.ExpiresAt.UTC(), LastUsedAt: m.LastUsedAt, LastUsedIP: m.LastUsedIP, RevokedAt: m.RevokedAt,
	}
	if err := json.Unmarshal(m.Scopes, &t.Scopes); err != nil {
		return entities.AccessToken{}, err
	}
	return t, json.Unmarshal(m.TeamIDs, &t.TeamIDs)
}
```

`app/identity/repositories/mysql/tokens.go`:

```go
package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errTokenNameTaken = apperror.Conflict("you already have a token with this name, pick another")
	errTokenNotFound  = apperror.NotFound("token not found")
)

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) TokenRepository {
	return TokenRepository{db: db}
}

func (r TokenRepository) Create(ctx context.Context, t entities.AccessToken) error {
	model, err := fromAccessToken(t)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errTokenNameTaken
	}
	return err
}

func (r TokenRepository) FindByHash(ctx context.Context, hash string) (entities.AccessToken, error) {
	return r.findWhere(ctx, entities.ErrTokenInvalid, "token_hash = ?", hash)
}

func (r TokenRepository) FindByID(ctx context.Context, id string) (entities.AccessToken, error) {
	return r.findWhere(ctx, errTokenNotFound, "id = ?", id)
}

func (r TokenRepository) findWhere(ctx context.Context, missing error, query, value string) (entities.AccessToken, error) {
	var model accessToken
	err := r.db.WithContext(ctx).Where(query, value).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.AccessToken{}, missing
	}
	if err != nil {
		return entities.AccessToken{}, err
	}
	return model.toEntity()
}

func (r TokenRepository) ListByOwner(ctx context.Context, ownerID string) ([]entities.AccessToken, error) {
	return list(r.db.WithContext(ctx).Where("owner_id = ?", ownerID))
}

func (r TokenRepository) ListAll(ctx context.Context) ([]entities.AccessToken, error) {
	return list(r.db.WithContext(ctx))
}

func list(tx *gorm.DB) ([]entities.AccessToken, error) {
	var models []accessToken
	if err := tx.Order("created_at DESC, id").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.AccessToken, 0, len(models))
	for _, m := range models {
		t, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}

func (r TokenRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&accessToken{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at).Error
}

func (r TokenRepository) Touch(ctx context.Context, id string, at time.Time, ip string) error {
	return r.db.WithContext(ctx).Model(&accessToken{}).Where("id = ?", id).
		Updates(map[string]any{"last_used_at": at, "last_used_ip": ip}).Error
}

func isDuplicate(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
```

The model has no `active_name` field on purpose: MySQL computes it, and GORM must never write it.

If `go vet` reports `duplicateEntry` or `isDuplicate` already declared in the identity `mysql` package, reuse the existing ones and delete these two declarations.

- [ ] **Step 9: Implement the audit columns and filters**

`app/audit/entities/entities.go`: add `TokenID string` and `TokenName string` after `ActorLogin` in `Event`; add `TokenID string` and `TeamIDs []string` after `ActorID` in `Filter`; add `Token string` after `Actor` in `Query`.

`app/audit/repositories/mysql/model.go`: add `TokenID string` and `TokenName string` after `ActorLogin` in `auditEvent`; in `fromEvent` and `toEntity` add `TokenID: e.TokenID, TokenName: e.TokenName` (and `m.TokenID`, `m.TokenName`) to the first line of each literal.

`app/audit/repositories/mysql/events.go`, replace `filtered`:

```go
func filtered(tx *gorm.DB, f entities.Filter) *gorm.DB {
	tx = whereSet(tx, "team_id = ?", f.TeamID)
	tx = whereSet(tx, "target_id = ?", f.TargetID)
	tx = whereSet(tx, "actor_id = ?", f.ActorID)
	tx = whereSet(tx, "token_id = ?", f.TokenID)
	tx = whereSet(tx, "environment = ?", f.Environment)
	if len(f.TeamIDs) > 0 {
		tx = tx.Where("team_id IN ?", f.TeamIDs)
	}
	if f.Before != nil {
		tx = tx.Where("at < ?", *f.Before)
	}
	return tx
}
```

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go test -p 1 ./pkg/... ./app/identity/... ./app/audit/... && go vet ./...`
Expected: PASS, no vet output.

- [ ] **Step 11: Commit the storage layer**

```bash
git add migrations/000005_access_tokens.up.sql migrations/000005_access_tokens.down.sql internal/testdb/testdb.go \
  pkg/scope pkg/apperror/apperror.go app/identity/entities/access_token.go app/identity/entities/access_token_test.go \
  app/identity/repositories/mysql/token_model.go app/identity/repositories/mysql/tokens.go app/identity/repositories/mysql/tokens_test.go \
  app/audit/entities/entities.go app/audit/repositories/mysql
git commit -m "Store access tokens and the token behind each audit event"
```

---

### Task 2: Rules — actor token, policy, errors, token on events

**Files:**
- Modify: `pkg/actor/actor.go`, `pkg/apperror/apperror.go`, `pkg/events/events.go`
- Modify: `app/authz/authz.go`; Create: `app/authz/token.go`, `app/authz/token_test.go`; Modify: `app/authz/authz_test.go` (`policy` var)
- Modify: `app/audit/usecases/record/record.go`
- Modify (token on events): `app/deploy/usecases/agents/agents.go:84`, `app/deploy/usecases/deployments/deployments.go:94`, `app/identity/usecases/users/users.go:74`, `app/catalog/usecases/services/services.go:57`, `app/envconfig/usecases/configs/revise.go:96`, `app/queue/usecases/queues/queues.go:114`, `app/release/usecases/releases/releases.go:89`, `app/team/usecases/members/change.go:90`, `app/team/usecases/teams/teams.go:118`
- Modify (new actions): `app/envconfig/usecases/configs/revise.go`, `app/envconfig/usecases/configs/secrets.go`, `app/queue/usecases/queues/edit.go`, `app/queue/usecases/queues/queues.go`
- Modify (wiring needed to compile): `cmd/controlplane/modules/authz.go`

**Interfaces:**
- Consumes: `scope.Scope`, `scope.*` from Task 1.
- Produces:

```go
package actor
type Token struct {
	ID, Name, Prefix string
	Scopes    []scope.Scope
	TeamIDs   []string
	Bypass    bool
	ExpiresAt time.Time
}
func (t Token) Has(s scope.Scope) bool
type Actor struct { UserID, Login string; Admin bool; Token Token }
func (a Actor) ViaToken() bool
func (a Actor) Sees(teamID string) bool
func (a Actor) SessionAdmin() bool

package apperror
const CodeMissingScope, CodeTeamNotGranted, CodeApprovalRequired Code
type AppError struct { Code; Message; RequiredScope string `json:"required_scope,omitempty"`; Team string `json:"team,omitempty"` }
func MissingScope(scopeName string) *AppError
func TeamNotGranted(teamID string) *AppError

package events
type Event struct { ...; TokenID, TokenName string }
const TargetToken = "token"

package authz
const EditSecrets Action = "secrets.edit"; EditQueue Action = "queue.edit"; ManageTokens Action = "tokens.manage"
type Publisher interface { Publish(ctx context.Context, event events.Event) }
func NewPolicy(memberships MembershipReader, environments ProtectionReader, publisher Publisher) Policy
func CanRead(a actor.Actor) error
```

- [ ] **Step 1: Write the failing policy tests**

In `app/authz/authz_test.go` change the `policy` var to pass a recorder, and add the recorder type:

```go
type recorder struct{ events []events.Event }

func (r *recorder) Publish(_ context.Context, e events.Event) { r.events = append(r.events, e) }

var policy = authz.NewPolicy(
	roles{"places/member": "member", "places/maintainer": "maintainer"},
	protection{"dev": false, "prod": true},
	&recorder{},
)
```

(import `github.com/klzn-labs/kalzone/pkg/events`.)

`app/authz/token_test.go`:

```go
package authz_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func via(owner string, scopes []scope.Scope, teams []string, bypass bool) actor.Actor {
	a := people[owner]
	a.Token = actor.Token{ID: "tok", Name: "claude-local", Scopes: scopes, TeamIDs: teams, Bypass: bypass}
	return a
}

func only(s ...scope.Scope) []scope.Scope { return s }

var places = []string{"places"}

type tokenCase struct {
	name   string
	actor  actor.Actor
	action authz.Action
	env    string
	code   apperror.Code
}

var tokenCases = []tokenCase{
	{"a member's deploy token deploys dev", via("member", only(scope.Deploy), places, false), authz.Deploy, "dev", ""},
	{"a missing scope is named", via("member", only(scope.Read), places, false), authz.Deploy, "dev", apperror.CodeMissingScope},
	{"a team not on the token", via("member", only(scope.Deploy), []string{"reviews"}, false), authz.Deploy, "dev", apperror.CodeTeamNotGranted},
	{"the owner lacks the role", via("outsider", scope.All, places, true), authz.Deploy, "dev", apperror.CodeForbidden},
	{"a member owner never reaches prod", via("member", scope.All, places, true), authz.Deploy, "prod", apperror.CodeForbidden},
	{"prod without protected", via("maintainer", only(scope.Deploy), places, false), authz.Deploy, "prod", apperror.CodeMissingScope},
	{"prod without bypass", via("maintainer", only(scope.Deploy, scope.Protected), places, false), authz.Deploy, "prod", apperror.CodeApprovalRequired},
	{"prod with bypass", via("maintainer", only(scope.Deploy, scope.Protected), places, true), authz.Deploy, "prod", ""},
	{"config on prod needs approval too", via("maintainer", only(scope.Config, scope.Protected), places, false), authz.EditConfig, "prod", apperror.CodeApprovalRequired},
	{"secrets need their own scope", via("member", only(scope.Config), places, false), authz.EditSecrets, "dev", apperror.CodeMissingScope},
	{"secrets with the secrets scope", via("member", only(scope.Secrets), places, false), authz.EditSecrets, "dev", ""},
	{"queues go with services", via("member", only(scope.Services), places, false), authz.EditQueue, "dev", ""},
	{"deleting needs services:admin", via("maintainer", only(scope.Services), places, false), authz.DeleteService, "", apperror.CodeMissingScope},
	{"builds", via("member", only(scope.Builds), places, false), authz.CreateRelease, "", ""},
	{"an admin owner is bound by scopes", via("admin", only(scope.Read), places, false), authz.EditService, "", apperror.CodeMissingScope},
	{"an admin owner is bound by teams", via("admin", scope.All, []string{"reviews"}, true), authz.EditService, "", apperror.CodeTeamNotGranted},
	{"an admin owner's token manages teams with teams:admin", via("admin", only(scope.TeamsAdmin), places, false), authz.ManageTeams, "", ""},
	{"admins.manage is never for tokens", via("admin", scope.All, places, true), authz.ManageAdmins, "", apperror.CodeForbidden},
	{"tokens.manage needs tokens:admin", via("outsider", only(scope.Read), places, false), authz.ManageTokens, "", apperror.CodeMissingScope},
	{"tokens.manage with tokens:admin", via("outsider", only(scope.TokensAdmin), places, false), authz.ManageTokens, "", ""},
}

func Test_Policy_Tokens(t *testing.T) {
	for _, c := range tokenCases {
		t.Run(c.name, checkToken(c))
	}
}

func checkToken(c tokenCase) func(t *testing.T) {
	return func(t *testing.T) {
		err := policy.Can(context.Background(), c.actor, c.action, authz.Resource{TeamID: "places", Environment: c.env})
		requireCode(t, c.code, err)
	}
}

func requireCode(t *testing.T, code apperror.Code, err error) {
	t.Helper()
	if code == "" {
		require.NoError(t, err)
		return
	}
	require.Equal(t, code, apperror.CodeOf(err), "%v", err)
}

func Test_Policy_SessionsManageTheirOwnTokens(t *testing.T) {
	err := policy.Can(context.Background(), people["outsider"], authz.ManageTokens, authz.Resource{})

	require.NoError(t, err)
}

func Test_Policy_ErrorsSayWhatIsMissing(t *testing.T) {
	ctx := context.Background()
	scopeErr := policy.Can(ctx, via("member", only(scope.Read), places, false), authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"})
	teamErr := policy.Can(ctx, via("member", only(scope.Deploy), []string{"reviews"}, false), authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"})

	require.Equal(t, "deploy", asAppError(t, scopeErr).RequiredScope)
	require.Equal(t, "this token lacks deploy", scopeErr.Error())
	require.Equal(t, "places", asAppError(t, teamErr).Team)
}

func asAppError(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	appErr, ok := err.(*apperror.AppError)
	require.True(t, ok, "%T", err)
	return appErr
}

func Test_Policy_BypassIsAuditedOnlyWhenUsed(t *testing.T) {
	rec := &recorder{}
	p := authz.NewPolicy(roles{"places/maintainer": "maintainer"}, protection{"dev": false, "prod": true}, rec)
	a := via("maintainer", only(scope.Deploy, scope.Protected), places, true)

	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "prod"}))
	require.NoError(t, p.Can(context.Background(), a, authz.Deploy, authz.Resource{TeamID: "places", Environment: "dev"}))

	require.Len(t, rec.events, 1)
	require.Equal(t, "token.bypass_used", rec.events[0].Action)
	require.Equal(t, "tok", rec.events[0].TokenID)
	require.Equal(t, "prod", rec.events[0].Environment)
	require.Equal(t, "deploy", rec.events[0].Details["action"])
}

func Test_CanRead_NeedsTheReadScopeOnlyFromAToken(t *testing.T) {
	require.NoError(t, authz.CanRead(people["member"]))
	require.NoError(t, authz.CanRead(via("member", only(scope.Read), places, false)))
	requireCode(t, apperror.CodeMissingScope, authz.CanRead(via("member", only(scope.Deploy), places, false)))
	requireCode(t, apperror.CodeMissingScope, authz.CanRead(via("member", only(scope.Logs), places, false)))
}
```

Also add `{authz.EditSecrets, "dev", ...}`, `{authz.EditSecrets, "prod", ...}`, `{authz.EditQueue, "dev", ...}`, `{authz.EditQueue, "prod", ...}` rows to the session `matrix` in `authz_test.go`, with the same `allowed` lists as the `EditConfig` rows.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./app/authz/`
Expected: FAIL to compile: `too many arguments in call to authz.NewPolicy`, `unknown field Token in struct literal`.

- [ ] **Step 3: Implement actor, errors and events**

`pkg/actor/actor.go`:

```go
package actor

import (
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Token struct {
	ID        string
	Name      string
	Prefix    string
	Scopes    []scope.Scope
	TeamIDs   []string
	Bypass    bool
	ExpiresAt time.Time
}

func (t Token) Has(s scope.Scope) bool {
	return slices.Contains(t.Scopes, s)
}

type Actor struct {
	UserID string
	Login  string
	Admin  bool
	Token  Token
}

func (a Actor) SignedIn() bool {
	return a.UserID != ""
}

func (a Actor) ViaToken() bool {
	return a.Token.ID != ""
}

func (a Actor) Sees(teamID string) bool {
	return !a.ViaToken() || slices.Contains(a.Token.TeamIDs, teamID)
}

func (a Actor) SessionAdmin() bool {
	return a.Admin && !a.ViaToken()
}
```

`pkg/apperror/apperror.go`: add codes

```go
	CodeMissingScope     Code = "err_missing_scope"
	CodeTeamNotGranted   Code = "err_team_not_granted"
	CodeApprovalRequired Code = "err_approval_required"
```

replace `AppError` and add constructors:

```go
type AppError struct {
	Code          Code   `json:"code"`
	Message       string `json:"message"`
	RequiredScope string `json:"required_scope,omitempty"`
	Team          string `json:"team,omitempty"`
}

func MissingScope(scopeName string) *AppError {
	return &AppError{Code: CodeMissingScope, Message: "this token lacks " + scopeName, RequiredScope: scopeName}
}

func TeamNotGranted(teamID string) *AppError {
	return &AppError{Code: CodeTeamNotGranted, Message: "this token is not granted team " + teamID, Team: teamID}
}
```

`pkg/events/events.go`: add `TokenID string` and `TokenName string` after `ActorLogin` in `Event`; add `TargetToken = "token"` to the target constants.

`app/audit/usecases/record/record.go`: in `Handle` add `TokenID: e.TokenID, TokenName: e.TokenName,` to the `entities.Event` literal.

In each of the nine publishers listed under **Files**, the literal line that reads `ActorID: a.UserID, ActorLogin: a.Login, ...` gets `TokenID: a.Token.ID, TokenName: a.Token.Name,` inserted right after `ActorLogin: a.Login,`. Example, `app/catalog/usecases/services/services.go`:

```go
func serviceEvent(a actor.Actor, action string, s entities.Service, details map[string]any) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetService, TargetID: s.ID, TargetName: s.Slug, TeamID: s.OwnerTeamID,
		Details: details,
	}
}
```

Confirm none is missed: `grep -rn "ActorLogin: a.Login" app --include=*.go | grep -v TokenID` must print nothing.

- [ ] **Step 4: Implement the policy**

`app/authz/authz.go`: add to the `Action` constants

```go
	EditSecrets     Action = "secrets.edit"
	EditQueue       Action = "queue.edit"
	ManageTokens    Action = "tokens.manage"
```

add the publisher dependency and replace `Can`, `requiredRole`:

```go
type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type Policy struct {
	memberships  MembershipReader
	environments ProtectionReader
	publisher    Publisher
}

func NewPolicy(memberships MembershipReader, environments ProtectionReader, publisher Publisher) Policy {
	return Policy{memberships: memberships, environments: environments, publisher: publisher}
}

func (p Policy) Can(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	if !a.SignedIn() {
		return errUnauthenticated
	}
	if err := p.ownerCan(ctx, a, action, r); err != nil {
		return err
	}
	if !a.ViaToken() {
		return nil
	}
	return p.tokenCan(ctx, a, action, r)
}

func (p Policy) ownerCan(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	if a.Admin || action == ManageTokens {
		return nil
	}
	required, err := p.requiredRole(ctx, action, r)
	if err != nil || required == RoleNone {
		return firstOr(err, errForbidden)
	}
	return p.requireRole(ctx, a, r.TeamID, required)
}

func (p Policy) requiredRole(ctx context.Context, action Action, r Resource) (Role, error) {
	switch action {
	case CreateService, EditService, CreateRelease:
		return RoleMember, nil
	case DeleteService, TransferService, ManageMembers:
		return RoleMaintainer, nil
	case EditConfig, EditSecrets, EditQueue, Deploy:
		return p.roleForEnvironment(ctx, r.Environment)
	}
	return RoleNone, nil
}
```

(import `github.com/klzn-labs/kalzone/pkg/events`.)

`app/authz/token.go`:

```go
package authz

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

var scopeOf = map[Action]scope.Scope{
	CreateService: scope.Services, EditService: scope.Services, EditQueue: scope.Services,
	DeleteService: scope.ServicesAdmin, TransferService: scope.ServicesAdmin,
	CreateRelease: scope.Builds, EditConfig: scope.Config, EditSecrets: scope.Secrets, Deploy: scope.Deploy,
	ManageMembers: scope.TeamsAdmin, ManageTeams: scope.TeamsAdmin, ManageTokens: scope.TokensAdmin,
}

var protectable = map[Action]bool{EditConfig: true, EditSecrets: true, EditQueue: true, Deploy: true}

var (
	errNeverForTokens   = apperror.Forbidden("no token can do this, sign in to the website")
	errApprovalRequired = apperror.New(apperror.CodeApprovalRequired, "changes to a protected environment need a human's approval")
)

func (p Policy) tokenCan(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	needed, allowed := scopeOf[action]
	if !allowed {
		return errNeverForTokens
	}
	if !a.Token.Has(needed) {
		return apperror.MissingScope(string(needed))
	}
	if r.TeamID != "" && !a.Sees(r.TeamID) {
		return apperror.TeamNotGranted(r.TeamID)
	}
	if !protectable[action] {
		return nil
	}
	return p.protectedRule(ctx, a, action, r)
}

func (p Policy) protectedRule(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	protected, err := p.environments.IsProtected(ctx, r.Environment)
	if err != nil || !protected {
		return err
	}
	if !a.Token.Has(scope.Protected) {
		return apperror.MissingScope(string(scope.Protected))
	}
	if !a.Token.Bypass {
		return errApprovalRequired
	}
	p.publisher.Publish(ctx, bypassEvent(a, action, r))
	return nil
}

func bypassEvent(a actor.Actor, action Action, r Resource) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name,
		Action: "token.bypass_used", TargetKind: events.TargetToken, TargetID: a.Token.ID, TargetName: a.Token.Name,
		TeamID: r.TeamID, Environment: r.Environment, Details: map[string]any{"action": string(action)},
	}
}

func CanRead(a actor.Actor) error {
	if a.ViaToken() && !a.Token.Has(scope.Read) {
		return apperror.MissingScope(string(scope.Read))
	}
	return nil
}
```

`cmd/controlplane/modules/authz.go`: add `func(b *eventbus.Bus) authz.Publisher { return b },` to the `fx.Provide` list (import `github.com/klzn-labs/kalzone/internal/eventbus`).

- [ ] **Step 5: Use the new actions**

`app/envconfig/usecases/configs/revise.go`: `editable` takes the action:

```go
func (u ConfigUsecase) editable(ctx context.Context, a actor.Actor, action authz.Action, serviceSlug, environment string) (target, entities.Revision, error) {
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return target{}, entities.Revision{}, err
	}
	resource := authz.Resource{TeamID: t.service.OwnerTeamID, Environment: environment}
	if err := u.authz.Can(ctx, a, action, resource); err != nil {
		return target{}, entities.Revision{}, err
	}
	current, err := u.current(ctx, t)
	return t, current, err
}
```

`Revise` calls `u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)`; `SetSecret` and `UnsetSecret` in `secrets.go` call `u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)` (add the `authz` import to `secrets.go`).

`app/queue/usecases/queues/edit.go` (`Create`) and `app/queue/usecases/queues/queues.go` (`editable`): replace `authz.EditConfig` with `authz.EditQueue`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go build ./... && go test ./app/authz/ ./app/envconfig/... ./app/queue/usecases/... ./app/audit/usecases/... ./pkg/...`
Expected: PASS. Existing unit fakes take any action, so they still pass.

---

### Task 3: Rules — token usecases

**Files:**
- Modify: `app/identity/entities/access_token.go` (add `Check`, `TokenAuthenticated`)
- Create: `app/identity/entities/token_draft.go`, `app/identity/entities/token_draft_test.go`
- Create: `app/identity/repositories/teams/teams.go`
- Create: `app/identity/usecases/tokens/tokens.go`, `grant.go`, `authenticate.go`
- Test: `app/identity/usecases/tokens/fakes_test.go`, `tokens_test.go`, `authenticate_test.go`

**Interfaces:**
- Consumes: `entities.AccessToken`, `NewTokenSecret`, `LooksLikeToken`, `ErrToken*`, `HashToken` (Task 1); `actor.Token`, `authz.ManageTokens`, `events.TargetToken` (Task 2).
- Produces:

```go
package entities
func (t AccessToken) Check(now time.Time) error
type TokenAuthenticated struct { User User; Token AccessToken }
func (t TokenAuthenticated) Actor() actor.Actor
type TokenDraft struct { Name string; Scopes, Teams []string; Bypass bool; ExpiresInDays int }
func (d TokenDraft) Check() ([]scope.Scope, error)
func (d TokenDraft) ExpiresAt(now time.Time) time.Time
type TokenGrant struct { Name string; Scopes []scope.Scope; TeamIDs []string; Bypass bool; ExpiresAt time.Time }
type IssuedToken struct { Token AccessToken; Secret string }
type Whoami struct { User User; Token actor.Token; Teams []string }

package teams
func NewLookup(teams TeamFinder) Lookup
func (l Lookup) IDBySlug(ctx context.Context, slug string) (string, error)
func (l Lookup) SlugsByIDs(ctx context.Context, ids []string) ([]string, error)

package tokens
func NewTokensUsecase(store TokenStore, users UserReader, teams TeamLookup, authz Authorizer, publisher Publisher) TokensUsecase
func (u TokensUsecase) Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error)
func (u TokensUsecase) List(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
func (u TokensUsecase) ListAll(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
func (u TokensUsecase) Revoke(ctx context.Context, a actor.Actor, id string) error
func (u TokensUsecase) Whoami(ctx context.Context, a actor.Actor) (entities.Whoami, error)
func (u TokensUsecase) Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error)
```

- [ ] **Step 1: Write the failing entity tests**

`app/identity/entities/token_draft_test.go`:

```go
package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var day = 24 * time.Hour

func draft() entities.TokenDraft {
	return entities.TokenDraft{Name: "claude-local", Scopes: []string{"read"}, Teams: []string{"places"}}
}

func Test_TokenDraft_Refusals(t *testing.T) {
	cases := map[string]func(*entities.TokenDraft){
		"empty name":      func(d *entities.TokenDraft) { d.Name = "" },
		"long name":       func(d *entities.TokenDraft) { d.Name = strings.Repeat("a", 65) },
		"spaces":          func(d *entities.TokenDraft) { d.Name = "my token" },
		"no scopes":       func(d *entities.TokenDraft) { d.Scopes = nil },
		"unknown scope":   func(d *entities.TokenDraft) { d.Scopes = []string{"root"} },
		"no teams":        func(d *entities.TokenDraft) { d.Teams = nil },
		"negative expiry": func(d *entities.TokenDraft) { d.ExpiresInDays = -1 },
		"over a year":     func(d *entities.TokenDraft) { d.ExpiresInDays = 366 },
	}
	for name, change := range cases {
		d := draft()
		change(&d)
		_, err := d.Check()
		require.ErrorIs(t, err, apperror.Invalid(""), name)
	}
}

func Test_TokenDraft_ExpiryDefaultsTo90Days(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	d := draft()
	year := draft()
	year.ExpiresInDays = 365

	require.Equal(t, now.Add(90*day), d.ExpiresAt(now))
	require.Equal(t, now.Add(365*day), year.ExpiresAt(now))
}

func Test_AccessToken_Check(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	live := entities.AccessToken{ExpiresAt: now.Add(time.Second)}
	expired := entities.AccessToken{ExpiresAt: now}
	revoked := entities.AccessToken{ExpiresAt: now.Add(day), RevokedAt: &now}

	require.NoError(t, live.Check(now))
	require.ErrorIs(t, expired.Check(now), entities.ErrTokenExpired)
	require.ErrorIs(t, revoked.Check(now), entities.ErrTokenRevoked)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./app/identity/entities/`
Expected: FAIL, `undefined: entities.TokenDraft`.

- [ ] **Step 3: Implement the draft and checks**

Append to `app/identity/entities/access_token.go` (add imports `github.com/klzn-labs/kalzone/pkg/actor`):

```go
func (t AccessToken) Check(now time.Time) error {
	if t.RevokedAt != nil {
		return ErrTokenRevoked
	}
	if !now.Before(t.ExpiresAt) {
		return ErrTokenExpired
	}
	return nil
}

type TokenAuthenticated struct {
	User  User
	Token AccessToken
}

func (t TokenAuthenticated) Actor() actor.Actor {
	return actor.Actor{UserID: t.User.ID, Login: t.User.Login, Admin: t.User.Admin, Token: actor.Token{
		ID: t.Token.ID, Name: t.Token.Name, Prefix: t.Token.Prefix, Scopes: t.Token.Scopes,
		TeamIDs: t.Token.TeamIDs, Bypass: t.Token.Bypass, ExpiresAt: t.Token.ExpiresAt,
	}}
}
```

`app/identity/entities/token_draft.go`:

```go
package entities

import (
	"fmt"
	"regexp"
	"time"

	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

const (
	DefaultTokenDays = 90
	MaxTokenDays     = 365
)

var tokenName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type TokenDraft struct {
	Name          string
	Scopes        []string
	Teams         []string
	Bypass        bool
	ExpiresInDays int
}

type TokenGrant struct {
	Name      string
	Scopes    []scope.Scope
	TeamIDs   []string
	Bypass    bool
	ExpiresAt time.Time
}

type IssuedToken struct {
	Token  AccessToken
	Secret string
}

type Whoami struct {
	User  User
	Token actor.Token
	Teams []string
}

func (d TokenDraft) Check() ([]scope.Scope, error) {
	if !tokenName.MatchString(d.Name) {
		return nil, apperror.Invalid("a token name is 1-64 letters, digits, dots, dashes or underscores")
	}
	if len(d.Teams) == 0 {
		return nil, apperror.Invalid("pick at least one team")
	}
	if d.ExpiresInDays < 0 || d.ExpiresInDays > MaxTokenDays {
		return nil, apperror.Invalid(fmt.Sprintf("a token expires within 1-%d days", MaxTokenDays))
	}
	if len(d.Scopes) == 0 {
		return nil, apperror.Invalid("pick at least one scope")
	}
	return scope.Parse(d.Scopes)
}

func (d TokenDraft) ExpiresAt(now time.Time) time.Time {
	days := d.ExpiresInDays
	if days == 0 {
		days = DefaultTokenDays
	}
	return now.Add(time.Duration(days) * 24 * time.Hour)
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test ./app/identity/entities/`
Expected: PASS.

- [ ] **Step 5: Write the failing usecase tests**

`app/identity/usecases/tokens/fakes_test.go`:

```go
package tokens_test

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	ana = actor.Actor{UserID: "u-ana", Login: "ana"}
	ops = actor.Actor{UserID: "u-ops", Login: "ops", Admin: true}
)

type store struct {
	byID    map[string]entities.AccessToken
	touched int
}

func (s *store) Create(_ context.Context, t entities.AccessToken) error {
	for _, other := range s.byID {
		if other.OwnerID == t.OwnerID && other.Name == t.Name && other.RevokedAt == nil {
			return apperror.Conflict("taken")
		}
	}
	s.byID[t.ID] = t
	return nil
}

func (s *store) FindByHash(_ context.Context, hash string) (entities.AccessToken, error) {
	for _, t := range s.byID {
		if t.Hash == hash {
			return t, nil
		}
	}
	return entities.AccessToken{}, entities.ErrTokenInvalid
}

func (s *store) FindByID(_ context.Context, id string) (entities.AccessToken, error) {
	t, ok := s.byID[id]
	if !ok {
		return entities.AccessToken{}, apperror.NotFound("token not found")
	}
	return t, nil
}

func (s *store) ListByOwner(context.Context, string) ([]entities.AccessToken, error) { return nil, nil }

func (s *store) ListAll(context.Context) ([]entities.AccessToken, error) { return nil, nil }

func (s *store) Revoke(_ context.Context, id string, at time.Time) error {
	t := s.byID[id]
	t.RevokedAt = &at
	s.byID[id] = t
	return nil
}

func (s *store) Touch(_ context.Context, id string, at time.Time, ip string) error {
	t := s.byID[id]
	t.LastUsedAt, t.LastUsedIP = &at, ip
	s.byID[id] = t
	s.touched++
	return nil
}

type users struct{}

func (users) FindByID(_ context.Context, id string) (entities.User, error) {
	return entities.User{ID: id, Login: id[2:]}, nil
}

type teams map[string]string

func (t teams) IDBySlug(_ context.Context, slug string) (string, error) {
	id, ok := t[slug]
	if !ok {
		return "", apperror.NotFound("team not found")
	}
	return id, nil
}

func (t teams) SlugsByIDs(_ context.Context, ids []string) ([]string, error) {
	result := []string{}
	for slug, id := range t {
		if contains(ids, id) {
			result = append(result, slug)
		}
	}
	return result, nil
}

func contains(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

type allow struct{}

func (allow) Can(_ context.Context, a actor.Actor, _ authz.Action, _ authz.Resource) error {
	if a.ViaToken() && !a.Token.Has(scope.TokensAdmin) {
		return apperror.MissingScope(string(scope.TokensAdmin))
	}
	return nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) { r.actions = append(r.actions, e.Action) }

type fixture struct {
	usecase tokens.TokensUsecase
	store   *store
	events  *recorder
}

func newFixture() fixture {
	clock.WithTime(now)
	s := &store{byID: map[string]entities.AccessToken{}}
	r := &recorder{}
	u := tokens.NewTokensUsecase(s, users{}, teams{"places": "t1", "reviews": "t2"}, allow{}, r)
	return fixture{usecase: u, store: s, events: r}
}

func agentOf(owner actor.Actor, scopes []scope.Scope, teamIDs []string, days int) actor.Actor {
	owner.Token = actor.Token{ID: "parent", Name: "parent", Scopes: scopes, TeamIDs: teamIDs, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour)}
	return owner
}

func draftOf(scopes ...string) entities.TokenDraft {
	return entities.TokenDraft{Name: "claude-local", Scopes: scopes, Teams: []string{"places"}}
}

func lasting(days int, d entities.TokenDraft) entities.TokenDraft {
	d.ExpiresInDays = days
	return d
}
```

`app/identity/usecases/tokens/tokens_test.go`:

```go
package tokens_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func Test_Create_IssuesAPlatTokenAndStoresOnlyItsHash(t *testing.T) {
	f := newFixture()

	issued, err := f.usecase.Create(ctx, ana, draftOf("read", "deploy"))

	require.NoError(t, err)
	stored := f.store.byID[issued.Token.ID]
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, issued.Secret)
	require.Equal(t, entities.HashToken(issued.Secret), stored.Hash)
	require.Equal(t, issued.Secret[5:13], stored.Prefix)
	require.Equal(t, "u-ana", stored.OwnerID)
	require.Equal(t, []string{"t1"}, stored.TeamIDs)
	require.Equal(t, []scope.Scope{scope.Read, scope.Deploy}, stored.Scopes)
	require.Equal(t, now.Add(90*24*time.Hour), stored.ExpiresAt)
	require.Equal(t, []string{"token.created"}, f.events.actions)
}

func Test_Create_NamesAreUniquePerOwner(t *testing.T) {
	f := newFixture()
	_, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)

	_, again := f.usecase.Create(ctx, ana, draftOf("read"))
	_, otherOwner := f.usecase.Create(ctx, ops, draftOf("read"))

	require.ErrorIs(t, again, apperror.Conflict(""))
	require.NoError(t, otherOwner)
}

func Test_Create_ARevokedTokenFreesItsName(t *testing.T) {
	f := newFixture()
	first, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)
	require.NoError(t, f.usecase.Revoke(ctx, ana, first.Token.ID))

	second, err := f.usecase.Create(ctx, ana, draftOf("read"))

	require.NoError(t, err)
	require.NotEqual(t, first.Token.ID, second.Token.ID)
}

func Test_Create_RejectsUnknownTeams(t *testing.T) {
	d := draftOf("read")
	d.Teams = []string{"ghosts"}

	_, err := newFixture().usecase.Create(ctx, ana, d)

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_BypassOnlyFromAnAdminSession(t *testing.T) {
	f := newFixture()
	d := draftOf("deploy", "protected")
	d.Bypass = true
	adminToken := agentOf(ops, scope.All, []string{"t1"}, 365)
	adminToken.Token.Bypass = true

	_, member := f.usecase.Create(ctx, ana, d)
	_, viaToken := f.usecase.Create(ctx, adminToken, d)
	issued, admin := f.usecase.Create(ctx, ops, d)

	require.ErrorIs(t, member, apperror.Forbidden(""))
	require.ErrorIs(t, viaToken, apperror.Forbidden(""))
	require.NoError(t, admin)
	require.True(t, issued.Token.Bypass)
}

func Test_Create_FromATokenNeverEscalates(t *testing.T) {
	f := newFixture()
	parent := agentOf(ana, []scope.Scope{scope.TokensAdmin, scope.Read}, []string{"t1"}, 30)
	otherTeam := draftOf("read")
	otherTeam.Teams = []string{"reviews"}

	_, scopeErr := f.usecase.Create(ctx, parent, lasting(30, draftOf("read", "deploy")))
	_, teamErr := f.usecase.Create(ctx, parent, lasting(30, otherTeam))
	child, err := f.usecase.Create(ctx, parent, lasting(31, draftOf("read")))

	require.ErrorIs(t, scopeErr, apperror.Forbidden(""))
	require.ErrorIs(t, teamErr, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, parent.Token.ExpiresAt, child.Token.ExpiresAt)
	require.Equal(t, "parent", f.store.byID[child.Token.ID].CreatedByTokenID)
}

func Test_Create_FromATokenNeedsTokensAdmin(t *testing.T) {
	_, err := newFixture().usecase.Create(ctx, agentOf(ana, []scope.Scope{scope.Read}, []string{"t1"}, 30), draftOf("read"))

	require.ErrorIs(t, err, apperror.MissingScope(""))
}

func Test_Revoke_OnlyTheOwnerOrAnAdminSession(t *testing.T) {
	f := newFixture()
	issued, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)
	adminToken := agentOf(ops, scope.All, []string{"t1"}, 30)

	stranger := f.usecase.Revoke(ctx, adminToken, issued.Token.ID)
	require.ErrorIs(t, stranger, apperror.NotFound(""))
	require.NoError(t, f.usecase.Revoke(ctx, ops, issued.Token.ID))
	require.NoError(t, f.usecase.Revoke(ctx, ana, issued.Token.ID))

	require.NotNil(t, f.store.byID[issued.Token.ID].RevokedAt)
	require.Equal(t, []string{"token.created", "token.revoked"}, f.events.actions)
}

func Test_ListAll_OnlyForAnAdminSession(t *testing.T) {
	f := newFixture()

	_, member := f.usecase.ListAll(ctx, ana)
	_, viaToken := f.usecase.ListAll(ctx, agentOf(ops, scope.All, []string{"t1"}, 30))
	_, admin := f.usecase.ListAll(ctx, ops)

	require.ErrorIs(t, member, apperror.Forbidden(""))
	require.ErrorIs(t, viaToken, apperror.Forbidden(""))
	require.NoError(t, admin)
}

func Test_Whoami_NamesTheTeams(t *testing.T) {
	who, err := newFixture().usecase.Whoami(ctx, agentOf(ana, []scope.Scope{scope.Read}, []string{"t1"}, 30))

	require.NoError(t, err)
	require.Equal(t, "ana", who.User.Login)
	require.Equal(t, []string{"places"}, who.Teams)
	require.Equal(t, "parent", who.Token.Name)
}
```

`app/identity/usecases/tokens/authenticate_test.go`:

```go
package tokens_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

func Test_Authenticate_ReturnsTheOwnerAndToken(t *testing.T) {
	f := newFixture()
	issued, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)

	got, err := f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.1")

	require.NoError(t, err)
	a := got.Actor()
	require.Equal(t, "u-ana", a.UserID)
	require.Equal(t, issued.Token.ID, a.Token.ID)
	require.True(t, a.ViaToken())
}

func Test_Authenticate_ReportsInvalidExpiredAndRevoked(t *testing.T) {
	f := newFixture()
	expiring, _ := f.usecase.Create(ctx, ana, entities.TokenDraft{Name: "short", Scopes: []string{"read"}, Teams: []string{"places"}, ExpiresInDays: 1})
	revoked, _ := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, f.usecase.Revoke(ctx, ana, revoked.Token.ID))

	_, garbage := f.usecase.Authenticate(ctx, "plat_nope", "")
	_, unknown := f.usecase.Authenticate(ctx, "plat_"+string(make([]byte, 43)), "")
	_, gone := f.usecase.Authenticate(ctx, revoked.Secret, "")
	clock.WithTime(now.Add(25 * time.Hour))
	_, late := f.usecase.Authenticate(ctx, expiring.Secret, "")

	require.ErrorIs(t, garbage, entities.ErrTokenInvalid)
	require.ErrorIs(t, unknown, entities.ErrTokenInvalid)
	require.ErrorIs(t, gone, entities.ErrTokenRevoked)
	require.ErrorIs(t, late, entities.ErrTokenExpired)
}

func Test_Authenticate_TouchesAtMostOncePerMinute(t *testing.T) {
	f := newFixture()
	issued, _ := f.usecase.Create(ctx, ana, draftOf("read"))

	for i := 0; i < 5; i++ {
		_, err := f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.1")
		require.NoError(t, err)
	}
	_, _ = f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.2")
	clock.WithTime(now.Add(2 * time.Minute))
	_, _ = f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.2")

	require.Equal(t, 3, f.store.touched)
	require.Equal(t, "10.0.0.2", f.store.byID[issued.Token.ID].LastUsedIP)
}
```

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./app/identity/usecases/tokens/`
Expected: FAIL, `no non-test Go files` / `undefined: tokens.NewTokensUsecase`.

- [ ] **Step 7: Implement the teams adapter**

`app/identity/repositories/teams/teams.go`:

```go
package teams

import (
	"context"

	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
	FindByIDs(ctx context.Context, ids []string) ([]team.Team, error)
}

type Lookup struct {
	teams TeamFinder
}

func NewLookup(teams TeamFinder) Lookup {
	return Lookup{teams: teams}
}

func (l Lookup) IDBySlug(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}

func (l Lookup) SlugsByIDs(ctx context.Context, ids []string) ([]string, error) {
	found, err := l.teams.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(found))
	for _, t := range found {
		slugs = append(slugs, t.Slug)
	}
	return slugs, nil
}
```

- [ ] **Step 8: Implement the usecase**

`app/identity/usecases/tokens/tokens.go`:

```go
package tokens

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type TokenStore interface {
	Create(ctx context.Context, t entities.AccessToken) error
	FindByHash(ctx context.Context, hash string) (entities.AccessToken, error)
	FindByID(ctx context.Context, id string) (entities.AccessToken, error)
	ListByOwner(ctx context.Context, ownerID string) ([]entities.AccessToken, error)
	ListAll(ctx context.Context) ([]entities.AccessToken, error)
	Revoke(ctx context.Context, id string, at time.Time) error
	Touch(ctx context.Context, id string, at time.Time, ip string) error
}

type UserReader interface {
	FindByID(ctx context.Context, id string) (entities.User, error)
}

type TeamLookup interface {
	IDBySlug(ctx context.Context, slug string) (string, error)
	SlugsByIDs(ctx context.Context, ids []string) ([]string, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

var (
	errOnlyAdmins    = apperror.Forbidden("only a platform admin in a browser session sees everyone's tokens")
	errTokenNotFound = apperror.NotFound("token not found")
)

type TokensUsecase struct {
	store     TokenStore
	users     UserReader
	teams     TeamLookup
	authz     Authorizer
	publisher Publisher
}

func NewTokensUsecase(store TokenStore, users UserReader, teams TeamLookup, authz Authorizer, publisher Publisher) TokensUsecase {
	return TokensUsecase{store: store, users: users, teams: teams, authz: authz, publisher: publisher}
}

func (u TokensUsecase) List(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error) {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return nil, err
	}
	return u.store.ListByOwner(ctx, a.UserID)
}

func (u TokensUsecase) ListAll(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error) {
	if !a.SessionAdmin() {
		return nil, errOnlyAdmins
	}
	return u.store.ListAll(ctx)
}

func (u TokensUsecase) Revoke(ctx context.Context, a actor.Actor, id string) error {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return err
	}
	t, err := u.store.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if t.OwnerID != a.UserID && !a.SessionAdmin() {
		return errTokenNotFound
	}
	if t.RevokedAt != nil {
		return nil
	}
	if err := u.store.Revoke(ctx, id, clock.Now().UTC()); err != nil {
		return err
	}
	u.publish(ctx, a, "token.revoked", t)
	return nil
}

func (u TokensUsecase) Whoami(ctx context.Context, a actor.Actor) (entities.Whoami, error) {
	user, err := u.users.FindByID(ctx, a.UserID)
	if err != nil {
		return entities.Whoami{}, err
	}
	teams, err := u.teams.SlugsByIDs(ctx, a.Token.TeamIDs)
	return entities.Whoami{User: user, Token: a.Token, Teams: teams}, err
}

func (u TokensUsecase) publish(ctx context.Context, a actor.Actor, action string, t entities.AccessToken) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetToken, TargetID: t.ID, TargetName: t.Name,
		Details: map[string]any{"owner_id": t.OwnerID, "scopes": t.Scopes, "bypass": t.Bypass},
	})
}
```

`app/identity/usecases/tokens/grant.go`:

```go
package tokens

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

var (
	errBypassNeedsAdmin = apperror.Forbidden("only a platform admin in a browser session can grant bypass")
	errEscalation       = apperror.Forbidden("a token can only create tokens with its own scopes and teams, or fewer")
)

func (u TokensUsecase) Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error) {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return entities.IssuedToken{}, err
	}
	grant, err := u.grantOf(ctx, d)
	if err != nil {
		return entities.IssuedToken{}, err
	}
	if grant, err = fitGrant(a, grant); err != nil {
		return entities.IssuedToken{}, err
	}
	issued, err := newIssued(a, grant)
	if err != nil {
		return entities.IssuedToken{}, err
	}
	if err := u.store.Create(ctx, issued.Token); err != nil {
		return entities.IssuedToken{}, err
	}
	u.publish(ctx, a, "token.created", issued.Token)
	return issued, nil
}

func (u TokensUsecase) grantOf(ctx context.Context, d entities.TokenDraft) (entities.TokenGrant, error) {
	scopes, err := d.Check()
	if err != nil {
		return entities.TokenGrant{}, err
	}
	teamIDs, err := u.teamIDs(ctx, d.Teams)
	if err != nil {
		return entities.TokenGrant{}, err
	}
	expiresAt := d.ExpiresAt(clock.Now().UTC())
	return entities.TokenGrant{Name: d.Name, Scopes: scopes, TeamIDs: teamIDs, Bypass: d.Bypass, ExpiresAt: expiresAt}, nil
}

func (u TokensUsecase) teamIDs(ctx context.Context, slugs []string) ([]string, error) {
	ids := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		id, err := u.teams.IDBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func fitGrant(a actor.Actor, g entities.TokenGrant) (entities.TokenGrant, error) {
	if g.Bypass && !a.SessionAdmin() {
		return g, errBypassNeedsAdmin
	}
	if !a.ViaToken() {
		return g, nil
	}
	if !subset(g.Scopes, a.Token.Scopes) || !subset(g.TeamIDs, a.Token.TeamIDs) {
		return g, errEscalation
	}
	if g.ExpiresAt.After(a.Token.ExpiresAt) {
		g.ExpiresAt = a.Token.ExpiresAt
	}
	return g, nil
}

func subset[T comparable](inner, outer []T) bool {
	for _, item := range inner {
		if !slices.Contains(outer, item) {
			return false
		}
	}
	return true
}

func newIssued(a actor.Actor, g entities.TokenGrant) (entities.IssuedToken, error) {
	secret, err := entities.NewTokenSecret()
	if err != nil {
		return entities.IssuedToken{}, err
	}
	token := entities.AccessToken{
		ID: uuid.NewUUID(), OwnerID: a.UserID, Name: g.Name, Hash: secret.Hash, Prefix: secret.Prefix,
		Scopes: g.Scopes, TeamIDs: g.TeamIDs, Bypass: g.Bypass, CreatedAt: clock.Now().UTC(),
		CreatedByTokenID: a.Token.ID, ExpiresAt: g.ExpiresAt,
	}
	return entities.IssuedToken{Token: token, Secret: secret.Raw}, nil
}
```

`app/identity/usecases/tokens/authenticate.go`:

```go
package tokens

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const touchEvery = time.Minute

func (u TokensUsecase) Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error) {
	if !entities.LooksLikeToken(raw) {
		return entities.TokenAuthenticated{}, entities.ErrTokenInvalid
	}
	t, err := u.store.FindByHash(ctx, entities.HashToken(raw))
	if err != nil {
		return entities.TokenAuthenticated{}, err
	}
	now := clock.Now().UTC()
	if err := t.Check(now); err != nil {
		return entities.TokenAuthenticated{}, err
	}
	user, err := u.users.FindByID(ctx, t.OwnerID)
	if err != nil {
		return entities.TokenAuthenticated{}, entities.ErrTokenInvalid
	}
	return entities.TokenAuthenticated{User: user, Token: t}, u.touchIfDue(ctx, t, now, ip)
}

func (u TokensUsecase) touchIfDue(ctx context.Context, t entities.AccessToken, now time.Time, ip string) error {
	if t.LastUsedAt != nil && now.Sub(*t.LastUsedAt) < touchEvery && t.LastUsedIP == ip {
		return nil
	}
	return u.store.Touch(ctx, t.ID, now, ip)
}
```

- [ ] **Step 9: Run them to verify they pass**

Run: `go test ./app/identity/...`
Expected: PASS.

---

### Task 4: Rules — reads limited to the token's teams

**Files:**
- Create: `app/catalog/usecases/services/visible.go`, `app/catalog/usecases/services/visible_test.go`
- Create: `app/envconfig/usecases/configs/visible.go`
- Create: `app/deploy/usecases/deployments/visible.go`
- Create: `app/release/usecases/releases/visible.go`
- Create: `app/queue/usecases/queues/visible.go`, `app/queue/usecases/queues/visible_test.go`
- Create: `app/audit/usecases/query/visible.go`; Modify: `app/audit/usecases/query/query_test.go`, `app/audit/repositories/lookups/lookups.go`

**Interfaces:**
- Consumes: `actor.Actor.Sees`, `ViaToken`, `Token.TeamIDs` (Task 2); `entities.Filter.TeamIDs`, `TokenID`, `Query.Token` (Task 1).
- Produces (handlers in Task 6 call these):

```go
func (u ServicesUsecase) GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Service, error)
func (u ServicesUsecase) ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Service, error)
func (u ConfigUsecase) CurrentFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Revision, error)
func (u ConfigUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Revision, error)
func (u ConfigUsecase) SecretsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) ([]entities.SecretInfo, error)
func (u DeploymentsUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Deployment, error)
func (u DeploymentsUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, []entities.Event, error)
func (u ReleasesUsecase) ListFor(ctx context.Context, a actor.Actor, serviceSlug string, limit int) ([]entities.Release, error)
func (u ReleasesUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Release, error)
func (u QueuesUsecase) GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error)
func (u QueuesUsecase) ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Queue, error)
func (u QueuesUsecase) VersionsFor(ctx context.Context, a actor.Actor, environment, name string) ([]entities.Version, error)
func (u QueuesUsecase) StatsFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Stats, bool, error)
func (u QueryUsecase) ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error)
```

- [ ] **Step 1: Write the failing tests**

`app/catalog/usecases/services/visible_test.go`:

```go
package services_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_GetFor_HidesServicesOfTeamsTheTokenLacks(t *testing.T) {
	_, _, _, usecase := setup(entities.Service{ID: "s1", Slug: "api", OwnerTeamID: "t1"})
	agent := ana
	agent.Token = actor.Token{ID: "tok", TeamIDs: []string{"t2"}}

	_, hidden := usecase.GetFor(ctx, agent, "api")
	found, err := usecase.GetFor(ctx, ana, "api")

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.NoError(t, err)
	require.Equal(t, "s1", found.ID)
}
```

`app/queue/usecases/queues/visible_test.go`:

```go
package queues_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ReadsFor_SkipQueuesOfOtherTeams(t *testing.T) {
	f := newFixture(t, false)
	f.store.queues[key("dev", "mine")] = entities.Queue{ID: "q1", Name: "mine", Environment: "dev", TeamID: "t1"}
	f.store.queues[key("dev", "theirs")] = entities.Queue{ID: "q2", Name: "theirs", Environment: "dev", TeamID: "t2"}
	agent := ana
	agent.Token = actor.Token{ID: "tok", TeamIDs: []string{"t1"}}

	list, err := f.usecase.ListFor(ctx, agent, "dev", "")
	require.NoError(t, err)
	_, hidden := f.usecase.GetFor(ctx, agent, "dev", "theirs")
	_, _, hiddenStats := f.usecase.StatsFor(ctx, agent, "dev", "theirs")
	all, _ := f.usecase.ListFor(ctx, ana, "dev", "")

	require.Len(t, list, 1)
	require.Equal(t, "mine", list[0].Name)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.ErrorIs(t, hiddenStats, apperror.NotFound(""))
	require.Len(t, all, 2)
}
```

Append to `app/audit/usecases/query/query_test.go` (add imports `github.com/klzn-labs/kalzone/pkg/actor`):

```go
type capture struct{ last entities.Filter }

func (c *capture) List(_ context.Context, f entities.Filter) ([]entities.Event, error) {
	c.last = f
	return nil, nil
}

func Test_ListFor_LimitsATokenToItsTeams(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)
	agent := actor.Actor{UserID: "u1", Token: actor.Token{ID: "tok", TeamIDs: []string{"t1"}}}

	_, err := usecase.ListFor(context.Background(), agent, entities.Filter{})
	require.NoError(t, err)
	viaToken := c.last
	_, _ = usecase.ListFor(context.Background(), actor.Actor{UserID: "u1"}, entities.Filter{})

	require.Equal(t, []string{"t1"}, viaToken.TeamIDs)
	require.Empty(t, c.last.TeamIDs)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./app/catalog/usecases/... ./app/queue/usecases/... ./app/audit/usecases/...`
Expected: FAIL, `usecase.GetFor undefined`, `f.usecase.ListFor undefined`.

- [ ] **Step 3: Implement the `…For` methods**

`app/catalog/usecases/services/visible.go`:

```go
package services

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("service not found")

func (u ServicesUsecase) GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Service, error) {
	s, err := u.store.FindBySlug(ctx, slug)
	if err != nil {
		return entities.Service{}, err
	}
	if !a.Sees(s.OwnerTeamID) {
		return entities.Service{}, errNotVisible
	}
	return s, nil
}

func (u ServicesUsecase) ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Service, error) {
	list, err := u.listBy(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(s entities.Service) bool { return !a.Sees(s.OwnerTeamID) }), nil
}

func (u ServicesUsecase) listBy(ctx context.Context, teamSlug string) ([]entities.Service, error) {
	if teamSlug == "" {
		return u.store.List(ctx, entities.ServiceFilter{})
	}
	return u.ListOwnedBy(ctx, teamSlug)
}
```

`app/envconfig/usecases/configs/visible.go`:

```go
package configs

import (
	"context"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("service not found")

func (u ConfigUsecase) visible(ctx context.Context, a actor.Actor, serviceSlug string) error {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return err
	}
	if !a.Sees(service.OwnerTeamID) {
		return errNotVisible
	}
	return nil
}

func (u ConfigUsecase) CurrentFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Revision, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return entities.Revision{}, err
	}
	return u.Current(ctx, serviceSlug, environment)
}

func (u ConfigUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Revision, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return nil, err
	}
	return u.History(ctx, serviceSlug, environment, limit)
}

func (u ConfigUsecase) SecretsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) ([]entities.SecretInfo, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return nil, err
	}
	return u.Secrets(ctx, serviceSlug, environment)
}
```

`app/deploy/usecases/deployments/visible.go`:

```go
package deployments

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("not found")

func (u DeploymentsUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Deployment, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return nil, errNotVisible
	}
	return u.History(ctx, serviceSlug, environment, limit)
}

func (u DeploymentsUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, []entities.Event, error) {
	d, logged, err := u.Get(ctx, id)
	if err != nil {
		return entities.Deployment{}, nil, err
	}
	service, err := u.services.FindServiceByID(ctx, d.ServiceID)
	if err != nil {
		return entities.Deployment{}, nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Deployment{}, nil, errNotVisible
	}
	return d, logged, nil
}
```

`app/release/usecases/releases/visible.go`:

```go
package releases

import (
	"context"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("not found")

func (u ReleasesUsecase) ListFor(ctx context.Context, a actor.Actor, serviceSlug string, limit int) ([]entities.Release, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return nil, errNotVisible
	}
	return u.List(ctx, serviceSlug, limit)
}

func (u ReleasesUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Release, error) {
	rel, err := u.Get(ctx, id)
	if err != nil {
		return entities.Release{}, err
	}
	service, err := u.services.FindServiceByID(ctx, rel.ServiceID)
	if err != nil {
		return entities.Release{}, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Release{}, errNotVisible
	}
	return rel, nil
}
```

`app/queue/usecases/queues/visible.go`:

```go
package queues

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("queue not found")

func (u QueuesUsecase) GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	if !a.Sees(q.TeamID) {
		return entities.Queue{}, errNotVisible
	}
	return q, nil
}

func (u QueuesUsecase) ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Queue, error) {
	list, err := u.List(ctx, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(q entities.Queue) bool { return !a.Sees(q.TeamID) }), nil
}

func (u QueuesUsecase) VersionsFor(ctx context.Context, a actor.Actor, environment, name string) ([]entities.Version, error) {
	if _, err := u.GetFor(ctx, a, environment, name); err != nil {
		return nil, err
	}
	return u.Versions(ctx, environment, name)
}

func (u QueuesUsecase) StatsFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Stats, bool, error) {
	if _, err := u.GetFor(ctx, a, environment, name); err != nil {
		return entities.Stats{}, false, err
	}
	return u.Stats(ctx, environment, name)
}
```

`app/audit/usecases/query/visible.go`:

```go
package query

import (
	"context"

	"github.com/klzn-labs/kalzone/app/audit/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

func (u QueryUsecase) ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error) {
	if a.ViaToken() {
		filter.TeamIDs = a.Token.TeamIDs
	}
	return u.List(ctx, filter)
}
```

`app/audit/repositories/lookups/lookups.go`: in `Resolve`, the first line becomes

```go
	filter := entities.Filter{Environment: q.Environment, TokenID: q.Token, Before: q.Before, Limit: q.Limit}
```

If a package already declares `errNotVisible`, `go vet` says so; rename the new one to `errHiddenByToken` in that package only.

- [ ] **Step 4: Run all unit tests to verify they pass**

Run: `go vet ./... && go test -p 1 ./app/... ./pkg/...`
Expected: PASS.

- [ ] **Step 5: Commit the rules layer (Tasks 2–4)**

```bash
git add pkg/actor pkg/apperror pkg/events app/authz app/audit app/identity/entities app/identity/repositories/teams \
  app/identity/usecases/tokens app/identity/usecases/users app/catalog/usecases app/envconfig/usecases \
  app/deploy/usecases app/release/usecases app/queue/usecases app/team/usecases cmd/controlplane/modules/authz.go
git status --short
git commit -m "Decide what an access token may do: owner, scopes, teams, protected environments and bypass"
```

`git status --short` must show nothing left unstaged from Tasks 2–4.

---

### Task 5: Handlers — Bearer authentication

**Files:**
- Modify: `app/identity/handlers/guard/guard.go`; Create: `app/identity/handlers/guard/bearer.go`
- Modify: `internal/httperror/httperror.go`
- Modify: `cmd/controlplane/modules/identity.go`
- Modify: `internal/integration/client.go`
- Create: `internal/integration/tests/tokens_test.go`

**Interfaces:**
- Consumes: `tokens.TokensUsecase.Authenticate`, `entities.TokenAuthenticated.Actor` (Task 3); `authz.CanRead` (Task 2).
- Produces:

```go
package guard
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error)
}
func NewGuards(sessions Authenticator, tokens TokenAuthenticator) httpserver.Guards

package integration
func (s *Suite) WithToken(t *testing.T, token string) *Client
func (c *Client) UseToken(token string)
```

- [ ] **Step 1: Add Bearer clients to the integration harness**

`internal/integration/client.go`: add `bearer string` to `Client`, and

```go
func (s *Suite) WithToken(t *testing.T, token string) *Client {
	t.Helper()
	c := s.Anonymous(t)
	c.bearer = token
	return c
}

func (c *Client) UseToken(token string) {
	c.bearer = token
}
```

In `Do`, after the CSRF header block:

```go
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
```

- [ ] **Step 2: Write the failing integration tests**

`internal/integration/tests/tokens_test.go`:

```go
package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

type issuedToken struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Prefix    string    `json:"prefix"`
	ExpiresAt time.Time `json:"expires_at"`
}

type whoami struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Token *struct {
		Name   string   `json:"name"`
		Prefix string   `json:"prefix"`
		Scopes []string `json:"scopes"`
		Teams  []string `json:"teams"`
		Bypass bool     `json:"bypass"`
	} `json:"token"`
}

func tokenBody(name string, scopes ...string) map[string]any {
	return map[string]any{"name": name, "scopes": scopes, "teams": []string{"places"}}
}

func issue(t *testing.T, c *integration.Client, body map[string]any) issuedToken {
	t.Helper()
	var out issuedToken
	c.MustJSON(t, http.MethodPost, "/api/v1/tokens", body, http.StatusCreated, &out)
	return out
}

func Test_ABearerTokenCallsTheAPIWithoutCSRF(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("claude-local", "read", "config")).Token)

	resp := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}})

	require.Equal(t, http.StatusCreated, resp.Status, resp.Body)
}

func Test_ABearerTokenIsRefusedOnPages(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("claude-local", "read")).Token)

	home := agent.Do(t, http.MethodGet, "/", nil)
	grafana := agent.Do(t, http.MethodGet, "/grafana/", nil)

	require.Equal(t, http.StatusUnauthorized, home.Status, home.Body)
	require.Contains(t, home.Body, "err_unauthenticated")
	require.Equal(t, http.StatusUnauthorized, grafana.Status, grafana.Body)
}

func Test_InvalidExpiredAndRevokedTokensGetTheirCodes(t *testing.T) {
	s, p := setupPlaces(t)
	revoked := issue(t, p.bob, tokenBody("revoked", "read"))
	expiring := issue(t, p.bob, map[string]any{"name": "short", "scopes": []string{"read"}, "teams": []string{"places"}, "expires_in_days": 1})
	p.bob.MustJSON(t, http.MethodDelete, "/api/v1/tokens/"+revoked.ID, nil, http.StatusNoContent, nil)

	garbage := s.WithToken(t, "plat_nope").Do(t, http.MethodGet, "/api/v1/whoami", nil)
	gone := s.WithToken(t, revoked.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)
	clock.WithTime(time.Now().Add(25 * time.Hour))
	t.Cleanup(func() { clock.Clock = nil })
	late := s.WithToken(t, expiring.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusUnauthorized, garbage.Status)
	require.Contains(t, garbage.Body, "err_token_invalid")
	require.Equal(t, http.StatusUnauthorized, gone.Status)
	require.Contains(t, gone.Body, "err_token_revoked")
	require.Equal(t, http.StatusUnauthorized, late.Status)
	require.Contains(t, late.Body, "err_token_expired")
}

func Test_BearerWinsOverTheSessionCookie(t *testing.T) {
	_, p := setupPlaces(t)
	miaToken := issue(t, p.mia, tokenBody("mia-agent", "read"))
	p.bob.UseToken(miaToken.Token)
	var who whoami

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, &who)

	require.Equal(t, "mia", who.Owner.Login)
	require.Equal(t, "mia-agent", who.Token.Name)
}

func sessionCookie(t *testing.T, s *integration.Suite, c *integration.Client) string {
	t.Helper()
	u, err := url.Parse(s.Server.URL)
	require.NoError(t, err)
	for _, cookie := range c.Cookies(u) {
		if cookie.Name == "platform_session" {
			return cookie.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func Test_MalformedAuthorizationHeaders(t *testing.T) {
	s, p := setupPlaces(t)
	cookie := "platform_session=" + sessionCookie(t, s, p.bob)
	whoamiWith := func(headers map[string]string) integration.Response {
		return s.Raw(t, http.MethodGet, "/api/v1/whoami", nil, headers)
	}

	sessionAsBearer := whoamiWith(map[string]string{"Authorization": "Bearer " + sessionCookie(t, s, p.bob)})
	lowercase := whoamiWith(map[string]string{"Authorization": "bearer plat_nope"})
	empty := whoamiWith(map[string]string{"Authorization": "Bearer "})
	basic := whoamiWith(map[string]string{"Authorization": "Basic Zm9vOmJhcg==", "Cookie": cookie})
	bearerAndCookie := whoamiWith(map[string]string{"Authorization": "Bearer plat_nope", "Cookie": cookie})

	require.Contains(t, sessionAsBearer.Body, "err_token_invalid")
	require.Contains(t, lowercase.Body, "err_token_invalid")
	require.Equal(t, http.StatusUnauthorized, empty.Status)
	require.Equal(t, http.StatusOK, basic.Status, basic.Body)
	require.Contains(t, bearerAndCookie.Body, "err_token_invalid")
}

func Test_AGetNeedsTheReadScopeExceptWhoami(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("deployer", "deploy")).Token)

	services := agent.Do(t, http.MethodGet, "/api/v1/services", nil)
	who := agent.Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusForbidden, services.Status)
	require.Contains(t, services.Body, `"code":"err_missing_scope"`)
	require.Contains(t, services.Body, `"required_scope":"read"`)
	require.Equal(t, http.StatusOK, who.Status, who.Body)
}
```

Add `"net/url"` to the imports of `tokens_test.go`. `Test_MalformedAuthorizationHeaders` sends a real session token as a Bearer token: it must be refused as `err_token_invalid`, never accepted as a session. `net/http` trims the trailing space of `Bearer `, so that case only asserts 401.

- [ ] **Step 3: Run them to verify they fail**

Run: `go test -p 1 ./internal/integration/... -run 'Bearer|Token|Malformed|ReadScope'`
Expected: FAIL: `POST /api/v1/tokens` answers 404 (route does not exist yet). The tokens API arrives in Task 6; these tests go green there. Continue with Step 4.

- [ ] **Step 4: Implement the guard**

`app/identity/handlers/guard/bearer.go`:

```go
package guard

import (
	"context"
	"strings"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/gin-gonic/gin"
)

const bearerScheme = "bearer "

type TokenAuthenticator interface {
	Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error)
}

var errBearerOnPages = apperror.New(apperror.CodeUnauthenticated, "access tokens work on /api/v1 only, sign in to use the website")

var readFree = map[string]bool{"/api/v1/whoami": true, "/api/v1/me": true, "/api/v1/tokens": true}

func bearer(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if len(header) < len(bearerScheme) || !strings.EqualFold(header[:len(bearerScheme)], bearerScheme) {
		return "", false
	}
	return strings.TrimSpace(header[len(bearerScheme):]), true
}

func authenticateToken(c *gin.Context, tokens TokenAuthenticator, raw string) error {
	authenticated, err := tokens.Authenticate(c.Request.Context(), raw, c.ClientIP())
	if err != nil {
		return err
	}
	a := authenticated.Actor()
	requestctx.SetViewer(c, requestctx.Viewer{Actor: a, Name: authenticated.User.Name, AvatarURL: authenticated.User.AvatarURL})
	if isSafe(c.Request.Method) && !readFree[c.FullPath()] {
		return authz.CanRead(a)
	}
	return nil
}
```

The scheme is matched case-insensitively, so `bearer plat_…` counts as Bearer. A header that is not Bearer at all (`Basic …`) is ignored and the session cookie is used.

`app/identity/handlers/guard/guard.go`: replace `NewGuards`, `apiGuard`, `webGuard`, `sessionGuard`:

```go
func NewGuards(sessions Authenticator, tokens TokenAuthenticator) httpserver.Guards {
	return httpserver.Guards{API: apiGuard(sessions, tokens), Web: webGuard(sessions), Session: sessionGuard(sessions)}
}

func apiGuard(sessions Authenticator, tokens TokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw, ok := bearer(c); ok {
			abortOn(c, authenticateToken(c, tokens, raw))
			return
		}
		if err := authenticate(c, sessions); err != nil {
			httperror.Abort(c, err)
			return
		}
		abortOn(c, checkCSRF(c))
	}
}

func webGuard(sessions Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := bearer(c); ok {
			httperror.Abort(c, errBearerOnPages)
			return
		}
		if err := authenticate(c, sessions); err != nil {
			redirectToLogin(c)
			return
		}
		abortOn(c, checkCSRF(c))
	}
}

func sessionGuard(sessions Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := bearer(c); ok {
			httperror.Abort(c, errBearerOnPages)
			return
		}
		if err := authenticate(c, sessions); err != nil {
			redirectToLogin(c)
		}
	}
}

func abortOn(c *gin.Context, err error) {
	if err != nil {
		httperror.Abort(c, err)
	}
}
```

`internal/httperror/httperror.go`: add to `statuses`

```go
	apperror.CodeMissingScope:     http.StatusForbidden,
	apperror.CodeTeamNotGranted:   http.StatusForbidden,
	apperror.CodeApprovalRequired: http.StatusForbidden,
	apperror.CodeTokenInvalid:     http.StatusUnauthorized,
	apperror.CodeTokenExpired:     http.StatusUnauthorized,
	apperror.CodeTokenRevoked:     http.StatusUnauthorized,
```

`cmd/controlplane/modules/identity.go`: provide the token pieces and inject them through a second function so neither body passes 20 lines:

```go
func Identity() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewUserRepository, mysql.NewSessionRepository, mysql.NewTokenRepository, github.NewClient,
			signin.NewSignInUsecase, sessions.NewSessionsUsecase, users.NewUsersUsecase, tokens.NewTokensUsecase,
			identityTeams.NewLookup, auth.NewAuthHandler, usersHandler.NewUsersHandler, guard.NewGuards,
		),
		identityInjections(),
		tokenInjections(),
		fx.Invoke(auth.RegisterAuthRoutes, me.RegisterMeRoutes, usersHandler.RegisterUsersRoutes),
	)
}

func tokenInjections() fx.Option {
	return fx.Provide(
		func(r mysql.TokenRepository) tokens.TokenStore { return r },
		func(r mysql.UserRepository) tokens.UserReader { return r },
		func(l identityTeams.Lookup) tokens.TeamLookup { return l },
		func(u teams.TeamsUsecase) identityTeams.TeamFinder { return u },
		func(p authz.Policy) tokens.Authorizer { return p },
		func(b *eventbus.Bus) tokens.Publisher { return b },
		func(u tokens.TokensUsecase) guard.TokenAuthenticator { return u },
	)
}
```

Imports: `identityTeams "github.com/klzn-labs/kalzone/app/identity/repositories/teams"`, `"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"`, `"github.com/klzn-labs/kalzone/app/team/usecases/teams"`.

- [ ] **Step 5: Build and run the existing suite**

Run: `go vet ./... && go test -p 1 ./internal/integration/... -run 'SignIn|CSRF|Logout|Callback|Routes'`
Expected: PASS: sessions and CSRF behave as before.

---

### Task 6: Handlers — tokens API, whoami, reads through the token's teams

**Files:**
- Create: `app/identity/handlers/tokens/tokens.go`, `app/identity/handlers/tokens/dto.go`, `app/identity/handlers/tokens/whoami.go`
- Modify: `app/catalog/handlers/services/services.go`, `app/envconfig/handlers/configs/configs.go`, `app/deploy/handlers/deployments/deployments.go`, `app/release/handlers/releases/releases.go`, `app/queue/handlers/queues/queues.go`, `app/queue/handlers/queues/observe.go`, `app/audit/handlers/audit/audit.go`
- Modify: `cmd/controlplane/modules/identity.go`, `cmd/controlplane/modules/audit.go`
- Create: `internal/integration/tests/token_access_test.go`

**Interfaces:**
- Consumes: `tokens.TokensUsecase` methods (Task 3); all `…For` methods (Task 4).
- Produces: routes `GET /api/v1/whoami`, `GET /api/v1/tokens[?all=true]`, `POST /api/v1/tokens`, `DELETE /api/v1/tokens/:id`; `GET /api/v1/audit?token=<id>`; audit event JSON gains `"token"` (token name).

- [ ] **Step 1: Write the failing integration tests**

`internal/integration/tests/token_access_test.go`:

```go
package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

type auditEvents struct {
	Events []struct {
		Action     string `json:"action"`
		Actor      string `json:"actor"`
		Token      string `json:"token"`
		TargetName string `json:"target_name"`
	} `json:"events"`
}

func deployVia(t *testing.T, c *integration.Client, releaseID, env string) integration.Response {
	t.Helper()
	return c.Do(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": env, "release_id": releaseID})
}

func agentFor(t *testing.T, s *integration.Suite, owner *integration.Client, body map[string]any) *integration.Client {
	t.Helper()
	return s.WithToken(t, issue(t, owner, body).Token)
}

func Test_ATokenDeploysToDevAndTheAuditNamesIt(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	bobToken := issue(t, p.bob, tokenBody("claude-local", "read", "deploy"))
	var trail auditEvents

	dev := deployVia(t, s.WithToken(t, bobToken.Token), releaseID, "dev")
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit?token="+bobToken.ID, nil, http.StatusOK, &trail)

	require.Equal(t, http.StatusAccepted, dev.Status, dev.Body)
	require.Equal(t, "deploy.requested", trail.Events[0].Action)
	require.Equal(t, "bob", trail.Events[0].Actor)
	require.Equal(t, "claude-local", trail.Events[0].Token)
}

func Test_ProdNeedsProtectedAndApprovalUnlessBypassed(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	bypass := map[string]any{"name": "root-bypass", "scopes": []string{"deploy", "protected"}, "teams": []string{"places"}, "bypass": true}
	var all auditEvents

	noScope := deployVia(t, agentFor(t, s, p.mia, tokenBody("mia-deploy", "deploy")), releaseID, "prod")
	noBypass := deployVia(t, agentFor(t, s, p.mia, tokenBody("mia-protected", "deploy", "protected")), releaseID, "prod")
	bypassed := deployVia(t, agentFor(t, s, p.root, bypass), releaseID, "prod")
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &all)

	require.Contains(t, noScope.Body, `"required_scope":"protected"`)
	require.Equal(t, http.StatusForbidden, noBypass.Status)
	require.Contains(t, noBypass.Body, "err_approval_required")
	require.Equal(t, http.StatusAccepted, bypassed.Status, bypassed.Body)
	require.True(t, hasAction(all, "token.bypass_used"))
}

func hasAction(list auditEvents, action string) bool {
	for _, e := range list.Events {
		if e.Action == action {
			return true
		}
	}
	return false
}

func hasTarget(list auditEvents, name string) bool {
	for _, e := range list.Events {
		if e.TargetName == name {
			return true
		}
	}
	return false
}

func reviewsService() map[string]string {
	return map[string]string{"slug": "reviews-api", "name": "Reviews", "owner_team": "reviews", "image_repository": "docker.io/adrianozdp/reviews-api"}
}

func createReviews(t *testing.T, p people) {
	t.Helper()
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "reviews", "name": "Reviews"}, http.StatusCreated, nil)
}

func Test_ScopeTeamAndRoleRefusalsAreDistinct(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPut, "/api/v1/teams/reviews/members/bob", map[string]string{"role": "member"}, http.StatusOK, nil)
	change := map[string]any{"set_env": map[string]string{"A": "1"}}

	scopeResp := agentFor(t, s, p.bob, tokenBody("ro", "read")).Do(t, http.MethodPost, "/api/v1/services", reviewsService())
	teamResp := agentFor(t, s, p.bob, tokenBody("places-only", "services")).Do(t, http.MethodPost, "/api/v1/services", reviewsService())
	roleResp := agentFor(t, s, p.eve, tokenBody("eve", "services", "config")).Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)
	secretResp := agentFor(t, s, p.bob, tokenBody("config-only", "config")).Do(t, http.MethodPut, "/api/v1/config/places-api/dev/secrets/DB_PASSWORD", map[string]string{"value": "x"})

	require.Contains(t, scopeResp.Body, `"required_scope":"services"`)
	require.Equal(t, http.StatusForbidden, teamResp.Status)
	require.Contains(t, teamResp.Body, "err_team_not_granted")
	require.Contains(t, roleResp.Body, "err_forbidden")
	require.Contains(t, secretResp.Body, `"required_scope":"secrets"`)
}

type serviceList struct {
	Services []struct {
		Slug string `json:"slug"`
	} `json:"services"`
}

func Test_ReadsOutsideTheTokensTeamsAreHidden(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	agent := agentFor(t, s, p.root, tokenBody("root-places", "read"))
	var list, other serviceList
	var events auditEvents

	agent.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &list)
	agent.MustJSON(t, http.MethodGet, "/api/v1/services?team=reviews", nil, http.StatusOK, &other)
	agent.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &events)

	require.Len(t, list.Services, 1)
	require.Equal(t, "places-api", list.Services[0].Slug)
	require.Empty(t, other.Services)
	require.False(t, hasTarget(events, "reviews-api"))
}

var reviewsReads = []string{
	"/api/v1/services/reviews-api", "/api/v1/config/reviews-api/dev", "/api/v1/config/reviews-api/dev/revisions",
	"/api/v1/config/reviews-api/dev/secrets", "/api/v1/releases?service=reviews-api", "/api/v1/deployments?service=reviews-api&env=dev",
}

func Test_OtherTeamsResourcesAnswer404(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	agent := agentFor(t, s, p.root, tokenBody("root-places", "read"))

	for _, path := range reviewsReads {
		require.Equal(t, http.StatusNotFound, agent.Do(t, http.MethodGet, path, nil).Status, path)
	}
}

func Test_LosingTheRoleTakesItFromTheToken(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("bob-config", "config")).Token)
	change := map[string]any{"set_env": map[string]string{"A": "1"}}
	before := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)

	p.mia.MustJSON(t, http.MethodDelete, "/api/v1/teams/places/members/bob", nil, http.StatusNoContent, nil)
	after := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)

	require.Equal(t, http.StatusCreated, before.Status, before.Body)
	require.Equal(t, http.StatusForbidden, after.Status)
	require.Contains(t, after.Body, "err_forbidden")
}

func Test_TokensCannotEscalate(t *testing.T) {
	s, p := setupPlaces(t)
	managerToken := issue(t, p.bob, tokenBody("manager", "tokens:admin", "read"))
	manager := s.WithToken(t, managerToken.Token)
	rootToken := agentFor(t, s, p.root, map[string]any{"name": "root-all", "scopes": []string{"tokens:admin", "read"}, "teams": []string{"places"}, "bypass": true})
	bypass := map[string]any{"name": "bypass", "scopes": []string{"read"}, "teams": []string{"places"}, "bypass": true}

	wider := manager.Do(t, http.MethodPost, "/api/v1/tokens", tokenBody("wider", "read", "deploy"))
	var narrower issuedToken
	manager.MustJSON(t, http.MethodPost, "/api/v1/tokens", tokenBody("narrower", "read"), http.StatusCreated, &narrower)
	memberBypass := p.bob.Do(t, http.MethodPost, "/api/v1/tokens", bypass)
	tokenBypass := rootToken.Do(t, http.MethodPost, "/api/v1/tokens", bypass)
	duplicate := p.bob.Do(t, http.MethodPost, "/api/v1/tokens", tokenBody("manager", "read"))

	require.Equal(t, http.StatusForbidden, wider.Status, wider.Body)
	require.WithinDuration(t, managerToken.ExpiresAt, narrower.ExpiresAt, time.Second)
	require.Equal(t, http.StatusForbidden, memberBypass.Status, memberBypass.Body)
	require.Equal(t, http.StatusForbidden, tokenBypass.Status, tokenBypass.Body)
	require.Equal(t, http.StatusConflict, duplicate.Status, duplicate.Body)
}

func Test_WhoamiAndTheSecretIsShownOnce(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("claude-local", "read", "deploy"))
	agent := s.WithToken(t, issued.Token)
	var who whoami

	agent.MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, &who)
	list := p.bob.Do(t, http.MethodGet, "/api/v1/tokens", nil)
	fromToken := agent.Do(t, http.MethodGet, "/api/v1/tokens", nil)

	require.Equal(t, "bob", who.Owner.Login)
	require.Equal(t, []string{"read", "deploy"}, who.Token.Scopes)
	require.Equal(t, []string{"places"}, who.Token.Teams)
	require.Equal(t, issued.Prefix, who.Token.Prefix)
	require.Equal(t, http.StatusOK, list.Status)
	require.Contains(t, list.Body, issued.Prefix)
	require.False(t, strings.Contains(list.Body, issued.Token))
	require.Contains(t, fromToken.Body, `"required_scope":"tokens:admin"`)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -p 1 ./internal/integration/... -run 'Token|Bearer|Malformed|ReadScope|Scope|Losing|Reads'`
Expected: FAIL, `POST /api/v1/tokens: 404`.

- [ ] **Step 3: Implement the tokens API**

`app/identity/handlers/tokens/dto.go`:

```go
package tokens

import (
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type createDTO struct {
	Name          string   `json:"name" binding:"required"`
	Scopes        []string `json:"scopes" binding:"required"`
	Teams         []string `json:"teams" binding:"required"`
	Bypass        bool     `json:"bypass"`
	ExpiresInDays int      `json:"expires_in_days"`
}

func (d createDTO) toEntity() entities.TokenDraft {
	return entities.TokenDraft{Name: d.Name, Scopes: d.Scopes, Teams: d.Teams, Bypass: d.Bypass, ExpiresInDays: d.ExpiresInDays}
}

type TokenDTO struct {
	ID         string        `json:"id"`
	OwnerID    string        `json:"owner_id"`
	Name       string        `json:"name"`
	Prefix     string        `json:"prefix"`
	Scopes     []scope.Scope `json:"scopes"`
	TeamIDs    []string      `json:"team_ids"`
	Bypass     bool          `json:"bypass"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	LastUsedAt *time.Time    `json:"last_used_at,omitempty"`
	LastUsedIP string        `json:"last_used_ip,omitempty"`
	RevokedAt  *time.Time    `json:"revoked_at,omitempty"`
}

type issuedDTO struct {
	TokenDTO
	Token string `json:"token"`
}

func ToDTO(t entities.AccessToken) TokenDTO {
	return TokenDTO{
		ID: t.ID, OwnerID: t.OwnerID, Name: t.Name, Prefix: t.Prefix, Scopes: t.Scopes, TeamIDs: t.TeamIDs,
		Bypass: t.Bypass, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt,
		LastUsedIP: t.LastUsedIP, RevokedAt: t.RevokedAt,
	}
}

type ownerDTO struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
}

type tokenInfoDTO struct {
	Name      string        `json:"name"`
	Prefix    string        `json:"prefix"`
	Scopes    []scope.Scope `json:"scopes"`
	Teams     []string      `json:"teams"`
	Bypass    bool          `json:"bypass"`
	ExpiresAt time.Time     `json:"expires_at"`
}

type whoamiDTO struct {
	Owner ownerDTO      `json:"owner"`
	Token *tokenInfoDTO `json:"token"`
}

func toWhoami(w entities.Whoami) whoamiDTO {
	result := whoamiDTO{Owner: ownerDTO{ID: w.User.ID, Login: w.User.Login, Name: w.User.Name, Admin: w.User.Admin}}
	if w.Token.ID == "" {
		return result
	}
	result.Token = &tokenInfoDTO{
		Name: w.Token.Name, Prefix: w.Token.Prefix, Scopes: w.Token.Scopes, Teams: w.Teams,
		Bypass: w.Token.Bypass, ExpiresAt: w.Token.ExpiresAt,
	}
	return result
}
```

`app/identity/handlers/tokens/tokens.go`:

```go
package tokens

import (
	"context"
	"net/http"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Tokens interface {
	List(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
	ListAll(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
	Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error)
	Revoke(ctx context.Context, a actor.Actor, id string) error
	Whoami(ctx context.Context, a actor.Actor) (entities.Whoami, error)
}

type TokensHandler struct {
	tokens Tokens
}

func NewTokensHandler(tokens Tokens) TokensHandler {
	return TokensHandler{tokens: tokens}
}

func (h TokensHandler) List(c *gin.Context) {
	list, err := h.list(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]TokenDTO, 0, len(list))
	for _, t := range list {
		result = append(result, ToDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": result})
}

func (h TokensHandler) list(c *gin.Context) ([]entities.AccessToken, error) {
	a := requestctx.ActorFrom(c)
	if c.Query("all") == "true" {
		return h.tokens.ListAll(c.Request.Context(), a)
	}
	return h.tokens.List(c.Request.Context(), a)
}

func (h TokensHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	issued, err := h.tokens.Create(c.Request.Context(), requestctx.ActorFrom(c), body.toEntity())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, issuedDTO{TokenDTO: ToDTO(issued.Token), Token: issued.Secret})
}

func (h TokensHandler) Revoke(c *gin.Context) {
	if err := h.tokens.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterTokensRoutes(api httpserver.API, h TokensHandler) {
	api.GET("/whoami", h.Whoami)
	api.GET("/tokens", h.List)
	api.POST("/tokens", h.Create)
	api.DELETE("/tokens/:id", h.Revoke)
}
```

`app/identity/handlers/tokens/whoami.go`:

```go
package tokens

import (
	"net/http"

	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/gin-gonic/gin"
)

func (h TokensHandler) Whoami(c *gin.Context) {
	who, err := h.tokens.Whoami(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toWhoami(who))
}
```

`cmd/controlplane/modules/identity.go`: add `tokensHandler.NewTokensHandler` to `fx.Provide`, `tokensHandler.RegisterTokensRoutes` to `fx.Invoke`, and `func(u tokens.TokensUsecase) tokensHandler.Tokens { return u },` to `tokenInjections` (import `tokensHandler "github.com/klzn-labs/kalzone/app/identity/handlers/tokens"`).

- [ ] **Step 4: Switch the JSON read handlers to the `…For` methods**

`app/catalog/handlers/services/services.go`: in the `Services` interface replace `List`, `ListOwnedBy` and `Get` with

```go
	ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Service, error)
	GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Service, error)
```

delete the `list` helper and use:

```go
func (h ServicesHandler) List(c *gin.Context) {
	list, err := h.services.ListFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("team"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]ServiceDTO, 0, len(list))
	for _, s := range list {
		result = append(result, ToDTO(s))
	}
	c.JSON(http.StatusOK, gin.H{"services": result})
}

func (h ServicesHandler) Get(c *gin.Context) {
	service, err := h.services.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	respond(c, http.StatusOK, service, err)
}
```

`app/envconfig/handlers/configs/configs.go`: interface methods `Current`, `History`, `Secrets` become `CurrentFor(ctx, a, serviceSlug, environment)`, `HistoryFor(ctx, a, serviceSlug, environment, limit)`, `SecretsFor(ctx, a, serviceSlug, environment)` with the signatures from Task 4; each handler passes `requestctx.ActorFrom(c)` as the second argument.

`app/deploy/handlers/deployments/deployments.go`: interface `History` → `HistoryFor(ctx, a, serviceSlug, environment, limit)`, `Get` → `GetFor(ctx, a, id)`; `List` and `Get` pass `requestctx.ActorFrom(c)`.

`app/release/handlers/releases/releases.go`: interface `List` → `ListFor(ctx, a, serviceSlug, limit)`, `Get` → `GetFor(ctx, a, id)`; handlers pass the actor.

`app/queue/handlers/queues/queues.go` and `observe.go`: interface `Get`, `List`, `Versions`, `Stats` → `GetFor`, `ListFor`, `VersionsFor`, `StatsFor` with an `a actor.Actor` second parameter; every caller (`List`, `Get`, `Stats`, `Versions`, `Dead`) passes `requestctx.ActorFrom(c)`.

`app/audit/handlers/audit/audit.go`:

```go
type Events interface {
	ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error)
}

type eventDTO struct {
	ID          string         `json:"id"`
	At          time.Time      `json:"at"`
	Actor       string         `json:"actor"`
	Action      string         `json:"action"`
	TargetKind  string         `json:"target_kind"`
	TargetName  string         `json:"target_name"`
	Environment string         `json:"environment,omitempty"`
	Details     map[string]any `json:"details"`
	Token       string         `json:"token,omitempty"`
}
```

In `List`, build each item with `eventDTO{e.ID, e.At, e.ActorLogin, e.Action, e.TargetKind, e.TargetName, e.Environment, e.Details, e.TokenName}` and call `h.list(c.Request.Context(), requestctx.ActorFrom(c), query)`; `list` takes the actor and calls `h.events.ListFor(ctx, a, filter)`. In `QueryFrom` add `Token: c.Query("token")` to the literal. `cmd/controlplane/modules/audit.go` keeps `func(u query.QueryUsecase) auditHandler.Events { return u }`; it compiles because `QueryUsecase` has `ListFor`.

- [ ] **Step 5: Run the integration tests to verify they pass**

Run: `go vet ./... && go test -p 1 ./internal/integration/...`
Expected: PASS, including Task 5's tests.

---

### Task 7: UI — Access tokens page, "via token" in the audit log

**Files:**
- Create: `app/web/handlers/tokens/tokens.go`, `app/web/templates/pages/tokens.html`
- Modify: `app/web/templates/layout.html`, `app/web/activity.go`, `app/web/templates/partials/components.html`, `app/web/handlers/audit/audit.go`, `app/web/templates/pages/audit.html`
- Modify: `cmd/controlplane/modules/web.go`
- Create: `internal/integration/tests/token_pages_test.go`

**Interfaces:**
- Consumes: `tokens.TokensUsecase` (Task 3), `web.Views.MyTeams`, `UsersByID`, `TeamsByID`.
- Produces: pages `GET /tokens`, `POST /tokens`, `POST /tokens/:id/revoke`.

- [ ] **Step 1: Write the failing page tests**

`internal/integration/tests/token_pages_test.go`:

```go
package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func tokenForm(csrf, name string) url.Values {
	return url.Values{"_csrf": {csrf}, "name": {name}, "scopes": {"read", "deploy"}, "teams": {"places"}, "expires_in_days": {"30"}}
}

func Test_TheTokensPageShowsTheSecretOnce(t *testing.T) {
	_, p := setupPlaces(t)

	created := p.bob.Form(t, "/tokens", tokenForm(p.bob.CSRFToken, "laptop"))
	later := p.bob.Do(t, http.MethodGet, "/tokens", nil)
	secret := secretFrom(t, created.Body)

	require.Equal(t, http.StatusOK, created.Status, created.Body)
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, secret)
	require.Contains(t, later.Body, "laptop")
	require.Contains(t, later.Body, secret[:13])
	require.False(t, strings.Contains(later.Body, secret))
}

func secretFrom(t *testing.T, body string) string {
	t.Helper()
	marker := `<code class="mono">`
	start := strings.Index(body, marker)
	require.GreaterOrEqual(t, start, 0, body)
	return body[start+len(marker) : start+len(marker)+48]
}

func Test_OnlyAdminsSeeBypassAndEveryonesTokens(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.Form(t, "/tokens", tokenForm(p.bob.CSRFToken, "bob-laptop"))

	member := p.bob.Do(t, http.MethodGet, "/tokens", nil)
	admin := p.root.Do(t, http.MethodGet, "/tokens", nil)

	require.NotContains(t, member.Body, `name="bypass"`)
	require.Contains(t, admin.Body, `name="bypass"`)
	require.Contains(t, admin.Body, "bob-laptop")
	require.Contains(t, admin.Body, "@bob")
}

func Test_RevokingFromThePage(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("to-revoke", "read"))

	resp := p.bob.Form(t, "/tokens/"+issued.ID+"/revoke", url.Values{"_csrf": {p.bob.CSRFToken}})
	after := s.WithToken(t, issued.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
	require.Contains(t, after.Body, "err_token_revoked")
}

func Test_TheAuditPageSaysViaWhichToken(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("claude-local", "read", "config"))
	s.WithToken(t, issued.Token).MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}}, http.StatusCreated, nil)

	page := p.root.Do(t, http.MethodGet, "/audit", nil)
	filtered := p.root.Do(t, http.MethodGet, "/audit?token="+issued.ID, nil)

	require.Contains(t, page.Body, "via")
	require.Contains(t, page.Body, "claude-local")
	require.Contains(t, filtered.Body, "changed the config of")
	require.NotContains(t, filtered.Body, "created team")
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -p 1 ./internal/integration/... -run 'TokensPage|Bypass|Revoking|AuditPage'`
Expected: FAIL, `/tokens` answers 404.

- [ ] **Step 3: Implement the page handler**

`app/web/handlers/tokens/tokens.go`:

```go
package tokens

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Tokens interface {
	List(ctx context.Context, a actor.Actor) ([]identity.AccessToken, error)
	ListAll(ctx context.Context, a actor.Actor) ([]identity.AccessToken, error)
	Create(ctx context.Context, a actor.Actor, d identity.TokenDraft) (identity.IssuedToken, error)
	Revoke(ctx context.Context, a actor.Actor, id string) error
}

type AllTeams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Row struct {
	Token    identity.AccessToken
	Owner    string
	Teams    []string
	Status   string
	LastUsed time.Time
}

type data struct {
	Mine       []Row
	Everyone   []Row
	Teams      []team.Team
	Scopes     []scope.Scope
	IsAdmin    bool
	Issued     string
	IssuedName string
}

type TokensHandler struct {
	renderer *web.Renderer
	views    web.Views
	tokens   Tokens
	teams    AllTeams
}

func NewTokensHandler(renderer *web.Renderer, views web.Views, tokens Tokens, teams AllTeams) TokensHandler {
	return TokensHandler{renderer: renderer, views: views, tokens: tokens, teams: teams}
}

func (h TokensHandler) Show(c *gin.Context) {
	h.render(c, identity.IssuedToken{})
}

func (h TokensHandler) Create(c *gin.Context) {
	issued, err := h.tokens.Create(c.Request.Context(), requestctx.ActorFrom(c), draftFrom(c))
	if err != nil {
		web.Fail(c, err)
		return
	}
	h.render(c, issued)
}

func (h TokensHandler) Revoke(c *gin.Context) {
	if err := h.tokens.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id")); err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/tokens", "Token revoked")
}

func draftFrom(c *gin.Context) identity.TokenDraft {
	days, _ := strconv.Atoi(c.PostForm("expires_in_days"))
	return identity.TokenDraft{
		Name: strings.TrimSpace(c.PostForm("name")), Scopes: c.PostFormArray("scopes"), Teams: c.PostFormArray("teams"),
		Bypass: c.PostForm("bypass") == "on", ExpiresInDays: days,
	}
}

func (h TokensHandler) render(c *gin.Context, issued identity.IssuedToken) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d.Issued, d.IssuedName = issued.Secret, issued.Token.Name
	h.renderer.Render(c, http.StatusOK, "tokens", web.Page{Title: "Access tokens", Nav: "tokens", Data: d})
}

func (h TokensHandler) build(ctx context.Context, a actor.Actor) (data, error) {
	mine, err := h.tokens.List(ctx, a)
	if err != nil {
		return data{}, err
	}
	d := data{Scopes: scope.All, IsAdmin: a.Admin}
	if d.Teams, err = h.teamChoices(ctx, a); err != nil {
		return data{}, err
	}
	if d.Mine, err = h.rows(ctx, mine); err != nil || !a.Admin {
		return d, err
	}
	everyone, err := h.tokens.ListAll(ctx, a)
	if err != nil {
		return data{}, err
	}
	d.Everyone, err = h.rows(ctx, everyone)
	return d, err
}

func (h TokensHandler) teamChoices(ctx context.Context, a actor.Actor) ([]team.Team, error) {
	if a.Admin {
		return h.teams.List(ctx)
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	teams := make([]team.Team, 0, len(mine))
	for _, m := range mine {
		teams = append(teams, m.Team)
	}
	return teams, nil
}

func (h TokensHandler) rows(ctx context.Context, list []identity.AccessToken) ([]Row, error) {
	owners, err := h.views.UsersByID(ctx, ownerIDs(list))
	if err != nil {
		return nil, err
	}
	teams, err := h.views.TeamsByID(ctx, teamIDs(list))
	if err != nil {
		return nil, err
	}
	now := clock.Now()
	result := make([]Row, 0, len(list))
	for _, t := range list {
		result = append(result, rowOf(t, owners[t.OwnerID].Login, teams, now))
	}
	return result, nil
}

func rowOf(t identity.AccessToken, owner string, teams map[string]team.Team, now time.Time) Row {
	row := Row{Token: t, Owner: owner, Teams: slugsOf(teams, t.TeamIDs), Status: statusOf(t, now)}
	if t.LastUsedAt != nil {
		row.LastUsed = *t.LastUsedAt
	}
	return row
}

func ownerIDs(list []identity.AccessToken) []string {
	ids := make([]string, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.OwnerID)
	}
	return ids
}

func teamIDs(list []identity.AccessToken) []string {
	ids := []string{}
	for _, t := range list {
		ids = append(ids, t.TeamIDs...)
	}
	return ids
}

func slugsOf(teams map[string]team.Team, ids []string) []string {
	slugs := make([]string, 0, len(ids))
	for _, id := range ids {
		slugs = append(slugs, teams[id].Slug)
	}
	return slugs
}

func statusOf(t identity.AccessToken, now time.Time) string {
	switch {
	case t.RevokedAt != nil:
		return "revoked"
	case !now.Before(t.ExpiresAt):
		return "expired"
	}
	return "active"
}

func RegisterTokensRoutes(routes httpserver.Web, h TokensHandler) {
	routes.GET("/tokens", h.Show)
	routes.POST("/tokens", h.Create)
	routes.POST("/tokens/:id/revoke", h.Revoke)
}
```

`cmd/controlplane/modules/web.go`: add `tokensPage.NewTokensHandler` to `fx.Provide` in `Web()`; add to `viewInjections` (or a new `tokenPageInjections()` included from `pageInjections`, if `viewInjections` would pass 20 lines):

```go
func tokenPageInjections() fx.Option {
	return fx.Provide(
		func(u tokens.TokensUsecase) tokensPage.Tokens { return u },
		func(u teams.TeamsUsecase) tokensPage.AllTeams { return u },
	)
}
```

and register `tokensPage.RegisterTokensRoutes` wherever `pageRoutes()` invokes the other `Register…Routes` (imports `tokensPage "github.com/klzn-labs/kalzone/app/web/handlers/tokens"`, `"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"`).

- [ ] **Step 4: Write the template and the menu link**

`app/web/templates/pages/tokens.html`:

```html
{{define "content"}}
{{$d := .Data}}{{$csrf := .Viewer.CSRFToken}}
<div class="page-header">
  <div>
    <div class="eyebrow">You</div>
    <h1>Access tokens</h1>
    <p class="lead">Tokens let agents and scripts call <span class="mono">/api/v1</span> as you, limited to the scopes and teams you pick. Send them as <span class="mono">Authorization: Bearer plat_…</span>.</p>
  </div>
</div>

{{if $d.Issued}}
<div class="alert alert-warn" style="margin-bottom:20px">{{icon "key"}}<span>Copy <b>{{$d.IssuedName}}</b> now, it is shown only once: <code class="mono">{{$d.Issued}}</code></span></div>
{{end}}

<form class="card" method="post" action="/tokens" style="margin-bottom:20px">
  {{template "csrf" $csrf}}
  <div class="card-body">
    <div class="form-row">
      <div class="field"><label for="name">Name</label><input id="name" name="name" type="text" class="mono" placeholder="claude-local" required maxlength="64" pattern="[A-Za-z0-9][A-Za-z0-9._-]{0,63}"></div>
      <div class="field"><label for="expires_in_days">Expires in</label>
        <select id="expires_in_days" name="expires_in_days">
          <option value="7">7 days</option><option value="30">30 days</option><option value="90" selected>90 days</option><option value="365">1 year</option>
        </select>
      </div>
    </div>
    <div class="field"><label>Scopes</label><div class="row">{{range $d.Scopes}}<label class="switch"><input type="checkbox" name="scopes" value="{{.}}">{{.}}</label>{{end}}</div></div>
    <div class="field"><label>Teams</label><div class="row">{{range $d.Teams}}<label class="switch"><input type="checkbox" name="teams" value="{{.Slug}}">{{.Slug}}</label>{{else}}<span class="muted">You are not on a team yet.</span>{{end}}</div></div>
    {{if $d.IsAdmin}}<div class="field"><label class="switch"><input type="checkbox" name="bypass">Bypass: changes to protected environments run without approval</label></div>{{end}}
    <div class="form-actions"><button class="btn btn-primary">{{icon "plus"}}Create token</button></div>
  </div>
</form>

<div class="card" style="margin-bottom:20px">
  <table class="table">
    <thead><tr><th>Token</th><th>Scopes</th><th>Teams</th><th>Expires</th><th>Last used</th><th class="right"></th></tr></thead>
    <tbody>{{range $d.Mine}}{{template "token-row" dict "Row" . "CSRF" $csrf "Owner" false}}{{else}}<tr><td colspan="6" class="muted">No tokens yet.</td></tr>{{end}}</tbody>
  </table>
</div>

{{if $d.IsAdmin}}
<h2>Everyone's tokens</h2>
<div class="card">
  <table class="table">
    <thead><tr><th>Token</th><th>Scopes</th><th>Teams</th><th>Expires</th><th>Last used</th><th class="right"></th></tr></thead>
    <tbody>{{range $d.Everyone}}{{template "token-row" dict "Row" . "CSRF" $csrf "Owner" true}}{{end}}</tbody>
  </table>
</div>
{{end}}
{{end}}

{{define "token-row"}}{{$r := .Row}}
<tr>
  <td><div><b class="mono">{{$r.Token.Name}}</b>{{if $r.Token.Bypass}} <span class="badge badge-warn">{{icon "shield"}}bypass</span>{{end}}</div><div class="sub mono">plat_{{$r.Token.Prefix}}…{{if .Owner}} · @{{$r.Owner}}{{end}}</div></td>
  <td><div class="row">{{range $r.Token.Scopes}}<span class="badge">{{.}}</span>{{end}}</div></td>
  <td><div class="row">{{range $r.Teams}}<span class="badge badge-accent">{{.}}</span>{{end}}</div></td>
  <td class="muted nowrap" title="{{date $r.Token.ExpiresAt}}">{{if eq $r.Status "active"}}{{date $r.Token.ExpiresAt}}{{else}}{{$r.Status}}{{end}}</td>
  <td class="muted nowrap">{{if not $r.LastUsed.IsZero}}{{ago $r.LastUsed}} · {{$r.Token.LastUsedIP}}{{else}}never{{end}}</td>
  <td class="right">{{if eq $r.Status "active"}}<form method="post" action="/tokens/{{$r.Token.ID}}/revoke" hx-post="/tokens/{{$r.Token.ID}}/revoke" hx-confirm="Revoke {{$r.Token.Name}}? Agents using it stop working at once.">{{template "csrf" .CSRF}}<button class="btn btn-sm btn-ghost">{{icon "trash"}}Revoke</button></form>{{end}}</td>
</tr>
{{end}}
```

`app/web/templates/layout.html`: in `.me`, before the sign-out `<form>`, add

```html
        <a class="btn btn-ghost btn-icon btn-sm {{if eq .Nav "tokens"}}active{{end}}" href="/tokens" title="Access tokens" aria-label="Access tokens">{{icon "key"}}</a>
```

- [ ] **Step 5: Show the token in the audit log**

`app/web/activity.go`: add `TokenID string` and `TokenName string` to `Activity`; in `activityOf` set them: `a := Activity{ActorLogin: e.ActorLogin, TokenID: e.TokenID, TokenName: e.TokenName, Phrase: e.Action, TargetName: e.TargetName, Environment: e.Environment, At: e.At}`; add phrasings

```go
	"token.created":       {"created access token", tokensLink, nil},
	"token.revoked":       {"revoked access token", tokensLink, nil},
	"token.bypass_used":   {"skipped approval with", nil, bypassDetail},
```

with

```go
func tokensLink(audit.Event) string { return "/tokens" }

func bypassDetail(e audit.Event) string {
	return fmt.Sprintf("%v on %s", e.Details["action"], e.Environment)
}
```

Add an `activity_test.go` case: an event with `TokenName: "claude-local"` keeps it on the `Activity`, and `token.bypass_used` with `Details{"action": "deploy"}`, `Environment: "prod"` has detail `deploy on prod`.

`app/web/templates/partials/components.html` line 18 becomes:

```html
    <div class="line"><b>@{{.ActorLogin}}</b>{{if .TokenName}} <span class="muted">via</span> <a class="mono" href="/audit?token={{.TokenID}}">{{.TokenName}}</a>{{end}} {{.Phrase}} {{if .Link}}<a href="{{.Link}}">{{.TargetName}}</a>{{else}}<b>{{.TargetName}}</b>{{end}}</div>
```

`app/web/handlers/audit/audit.go` `queryFrom`: add `Token: c.Query("token")` to the literal. `app/web/templates/pages/audit.html`: inside the filter form add `{{if $d.Query.Token}}<input type="hidden" name="token" value="{{$d.Query.Token}}">{{end}}`, and append `&token={{$d.Query.Token}}` to the "Older events" link.

- [ ] **Step 6: Run everything to verify it passes**

Run: `make lint && make test`
Expected: PASS, `gofmt` clean.

- [ ] **Step 7: Commit the handlers layer (Tasks 5–7)**

```bash
git add app/identity/handlers app/catalog/handlers app/envconfig/handlers app/deploy/handlers app/release/handlers \
  app/queue/handlers app/audit/handlers app/web internal/httperror internal/integration cmd/controlplane/modules
git status --short
git commit -m "Accept access tokens on the API and manage them from the API and the website"
```

`git status --short` must show only `docs/` changes or nothing.

---

### Task 8: Docs — README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add the section**

In "What is in the core", after the sign-in bullet add:

```markdown
- Access tokens for agents and scripts: Bearer tokens owned by a person, limited by scopes and teams
```

Add after the "Configuration" section:

````markdown
## Access tokens

Agents call the API with a token instead of a browser session. Create one under
**Access tokens** (key icon next to your name) or with the API:

```
curl -H "Authorization: Bearer $PLATFORM_TOKEN" https://platform.kalz.one/api/v1/whoami
```

A token acts for the person who made it and can do at most what that person, its
scopes and its teams all allow. Scopes: `read`, `logs` (log endpoints come later;
`read` does not include it), `services`, `services:admin`, `builds`, `config`,
`secrets`, `deploy`, `protected`, `teams:admin`, `tokens:admin`. Teams are listed
explicitly. Revoking a token frees its name; a token made by another token never
outlives it. Tokens expire after 1 to 365 days (90 by default) and
are shown once; only a hash is stored.

On a protected environment (`prod`) a token needs `protected`, and then gets
`err_approval_required` unless a platform admin gave it bypass from the website.
Tokens are refused on website pages and need no CSRF header. Every change made
with a token shows in the audit log as "person via token".

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/whoami` | owner, scopes, teams, bypass and expiry of the calling token |
| `GET /api/v1/tokens` | your tokens (`?all=true` for admins in the browser) |
| `POST /api/v1/tokens` | `{"name","scopes","teams","expires_in_days","bypass"}` → the token, once |
| `DELETE /api/v1/tokens/:id` | revoke |
````

- [ ] **Step 2: Commit the docs**

```bash
git add README.md
git commit -m "Document access tokens"
```

- [ ] **Step 3: Check the branch shape**

Run: `git log --oneline -5`
Expected: four new commits on top of the plan commit, in the order storage, rules, handlers, docs.

---

## Self-review notes

- Spec §3 owner ∩ scopes ∩ teams, admin not inherited: Task 2 (`tokenCases`). §4 scope table and protected table: Task 2; reads by team: Task 4, Task 6. §5 format, storage, teams, expiry, who creates what: Tasks 1, 3, 6. §6 Bearer, no CSRF, Bearer wins, pages refuse, 401 codes, `actor.Token`: Tasks 2, 3, 5. §7 whoami, tokens API, UI: Tasks 6, 7. §8 errors: Tasks 2, 5, 6. §9 audit columns, "via", filter, token events, bypass events: Tasks 1, 2, 3, 7. §10 migration: Task 1. §11 tests: Tasks 2, 3, 5, 6. §12 done-when: `Test_ATokenDeploysToDevAndTheAuditNamesIt` and `Test_ProdNeedsProtectedAndApprovalUnlessBypassed`.
- `err_team_not_granted` names the team by id: the policy cannot look up slugs without depending on the teams usecase, which depends on the policy.

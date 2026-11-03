# Local Accounts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Kalzone without GitHub settings signs people in with username and password, starting from a one-time setup link, with invites, resets and removal; a Kalzone with GitHub settings works exactly as today.

**Architecture:** New storage for password hashes, disabled users and one-time account links; a new `app/identity/usecases/accounts` package holding the mode, setup, password sign-in, invites and people rules; web pages, API operations, MCP tools and a `controlplane setup-link` subcommand on top. Writes that the spec requires to be atomic go through one identity repository that runs a single gorm transaction.

**Tech Stack:** Go 1.26, gin, htmx templates, gorm + MySQL 8, Uber fx, `golang.org/x/crypto/argon2`, testify, the `internal/integration` suite.

**Spec:** `docs/superpowers/specs/2026-11-03-local-accounts-design.md`

## Global Constraints

- Mode: GitHub mode when `cfg.GitHub.ClientID != "" && cfg.GitHub.ClientSecret != ""`, password mode otherwise. Every local-account path is off in GitHub mode.
- Passwords: 12 to 128 characters, no composition rules; argon2id in PHC string format (`$argon2id$v=19$m=…,t=…,p=…$salt$hash`), parameters stored with the hash.
- Usernames: `^[a-z0-9-]{1,39}$`, stored lowercase in `users.login`.
- Link lifetimes: setup 24h, invite 7 days, reset 24h. Links are `<base>/join/<token>`, token = `entities.NewToken()`, stored only as `entities.HashToken(token)`.
- Invalid, used, revoked or expired link → the same page with "This link is no longer valid."
- Sign-in failure message, identical for every cause: "Wrong username or password." Throttled: "Too many attempts, try again later."
- Rate limits count failures only: 10 per IP per hour, 10 per (username, IP) per hour; never per username alone.
- GitHub-mode refusal for invite and reset operations: `409 err_github_mode`, hint "this Kalzone signs people in with GitHub; add them to the team after they sign in".
- `PLATFORM_ADMINS` is applied only in GitHub sign-in.
- No password, hash or link token in any log line, audit payload or response after its first display.
- Repository rules from `~/.claude/CLAUDE.md` apply: functions ≤ 20 lines, ≤ 2 nesting levels, no comments.

## Review Focus

1. Uppercase or padded usernames at sign-in (`" Ana "`) — expected to sign in as `ana`; test in Task 4 `Test_SignInNormalizesTheUsername`.
2. The setup link logged at start on a restart before anyone used it — expected: the old link stops working and the new one works; test in Task 3 `Test_EnsureSetupLinkRevokesTheEarlierOne`.
3. An invite opened by someone already signed in — expected "Sign out to accept this invite", no user created; test in Task 7 `Test_InviteAsksASignedInPersonToSignOut`.
4. A removed person's open browser tab posting a form — expected redirect to `/login`, nothing written; test in Task 7 `Test_RemovedPersonIsSignedOutEverywhere`.
5. A password of exactly 12 and exactly 128 characters, and 129 — accepted, accepted, refused; test in Task 1 `Test_PasswordRuleBounds`.

---

### Task 1: Passwords and the account entities

**Files:**
- Create: `app/identity/entities/password.go`, `app/identity/entities/password_test.go`
- Create: `app/identity/entities/account_link.go`, `app/identity/entities/account_link_test.go`
- Modify: `app/identity/entities/entities.go` (User gains `PasswordHash string`, `DisabledAt *time.Time`, method `Disabled() bool`, `Local() bool` = `PasswordHash != ""`)
- Modify: `go.mod` (`golang.org/x/crypto` is already indirect; make it direct)

**Interfaces:**
- Produces:
  - `func HashPassword(password string) (string, error)`
  - `func VerifyPassword(encoded, password string) bool` (constant-time compare of the derived key)
  - `func CheckPassword(password string) error` (`apperror.Invalid` with field `password`, message "use 12 to 128 characters")
  - `func NormalizeLogin(login string) (string, error)` (trim, lowercase, `^[a-z0-9-]{1,39}$`, field `username`)
  - `var DummyHash string` (a hash of a random password, computed once in `init`, used for unknown usernames)
  - `type LinkKind string` with `LinkSetup`, `LinkInvite`, `LinkReset`
  - `type AccountLink struct { ID string; Kind LinkKind; TokenHash string; TeamID string; Role string; UserID string; CreatedBy string; CreatedAt, ExpiresAt time.Time; UsedAt, RevokedAt *time.Time }`
  - `func (l AccountLink) Live(now time.Time) bool`
  - `func NewAccountLink(kind LinkKind, now time.Time) (AccountLink, string, error)` — returns the link with ID, hash and expiry by kind, and the plain token.

- [ ] **Step 1: Write the failing tests**

`Test_PasswordRoundTrip` (hash then verify true; verify with another password false; two hashes of one password differ), `Test_PasswordRuleBounds` (11 → error, 12 → nil, 128 → nil, 129 → error), `Test_NormalizeLogin` (`" Ana "` → `"ana"`; `"a.b"`, `""`, 40 chars → error), `Test_NewAccountLinkExpiry` (setup and reset now+24h, invite now+168h; `TokenHash == HashToken(token)`), `Test_LiveLink` (used, revoked, expired → false).

- [ ] **Step 2: Run** `go test ./app/identity/entities/ -run 'Password|Login|AccountLink|LiveLink'` — expect compile failure.

- [ ] **Step 3: Implement** — argon2id with `time=2, memory=64*1024, threads=1, keyLen=32`, 16-byte salt from `crypto/rand`, `subtle.ConstantTimeCompare`.

- [ ] **Step 4: Run** the same command — expect PASS.

- [ ] **Step 5: Commit** `git commit -m "Hash passwords and model one-time account links"`

### Task 2: Storage — migration and repositories

**Files:**
- Create: `migrations/000017_local_accounts.up.sql`, `migrations/000017_local_accounts.down.sql`
- Modify: `app/identity/repositories/mysql/model.go` (user gains `PasswordHash *string`, `DisabledAt *time.Time`)
- Create: `app/identity/repositories/mysql/account_links.go`, `app/identity/repositories/mysql/accounts.go`, `app/identity/repositories/mysql/accounts_test.go`
- Modify: `app/identity/repositories/mysql/sessions.go`, `app/identity/repositories/mysql/tokens.go`

Migration up: `ALTER TABLE users ADD password_hash VARCHAR(255) NULL, ADD disabled_at DATETIME NULL;` and `CREATE TABLE account_links (id CHAR(36) PK, kind VARCHAR(16) NOT NULL, token_hash CHAR(64) NOT NULL UNIQUE, team_id CHAR(36) NULL, role VARCHAR(16) NULL, user_id CHAR(36) NULL, created_by CHAR(36) NULL, created_at DATETIME NOT NULL, expires_at DATETIME NOT NULL, used_at DATETIME NULL, revoked_at DATETIME NULL, INDEX account_links_team (team_id))`. Down reverses both.

**Interfaces:**
- Produces on `AccountLinkRepository` (`NewAccountLinkRepository(db *gorm.DB)`):
  - `Create(ctx, entities.AccountLink) error`
  - `FindByHash(ctx, hash string) (entities.AccountLink, error)` (NotFound when absent)
  - `RevokeUnused(ctx, kind entities.LinkKind, at time.Time) error`
  - `ListInvites(ctx, teamID string, now time.Time) ([]entities.AccountLink, error)` (live only, newest first)
  - `Revoke(ctx, id string, at time.Time) error`
- Produces on `AccountRepository` (`NewAccountRepository(db *gorm.DB)`), each one gorm transaction that first runs `UPDATE account_links SET used_at=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?` and returns `errLinkGone` (`apperror.NotFound("This link is no longer valid.")`) when 0 rows changed:
  - `CompleteSetup(ctx, linkID string, owner entities.User, team Team) error` — inserts the user, the team row (`teams`: id, slug `default`, name `Default`, created_at) and a `maintainer` membership.
  - `AcceptInvite(ctx, link entities.AccountLink, user entities.User) error` — inserts the user and the membership `(link.TeamID, user.ID, link.Role)`.
  - `ResetPassword(ctx, link entities.AccountLink, hash string) error` — sets `users.password_hash`, deletes the user's sessions.
  - `Disable(ctx, userID string, at time.Time) error` — deletes memberships and sessions, sets `revoked_at` on live tokens, clears `is_admin`, sets `disabled_at` (the Grafana roster then drops them on its next run).
  - `CountPeople(ctx) (int, error)` — users with `kind = 'person'`.
  - `type Team struct { ID, Slug, Name string; CreatedAt time.Time }`
- Produces `SessionRepository.DeleteByUser(ctx, userID, keepHash string) error` and `UserRepository.SetPassword(ctx, userID, hash string) error`.

- [ ] **Step 1: Write the failing tests** (repository tests on the MySQL test DSN, as `users_test.go` does): `Test_SetupClaimsTheLinkOnce` (two concurrent `CompleteSetup` on one link → exactly one nil error, one user row), `Test_DisableEndsAccess` (memberships 0, sessions 0, tokens revoked, `disabled_at` set), `Test_RevokeUnusedLeavesUsedLinks`, `Test_ListInvitesSkipsDeadLinks`.
- [ ] **Step 2: Run** `go test -p 1 ./app/identity/repositories/mysql/ -run 'Setup|Disable|Unused|ListInvites'` — expect failure.
- [ ] **Step 3: Implement** the migration, models and methods.
- [ ] **Step 4: Run** the same command — expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Store password hashes, disabled users and account links"`

### Task 3: Rules — mode and setup

**Files:**
- Create: `app/identity/usecases/accounts/mode.go`, `setup.go`, `setup_test.go`, `fakes_test.go`
- Modify: `pkg/apperror/apperror.go` (add `CodeGitHubMode Code = "err_github_mode"`)

**Interfaces:**
- Consumes: Task 1 entities, Task 2 repositories through interfaces declared in this package (`LinkStore`, `AccountStore`).
- Produces:
  - `type Mode struct{ GitHub bool }`, `func NewMode(cfg config.Config) Mode`, `func (m Mode) RequirePasswords() error` (the 409 from Global Constraints)
  - `type SetupUsecase`, `NewSetupUsecase(cfg config.Config, mode Mode, links LinkStore, accounts AccountStore, tokens TokenIssuer, logger *slog.Logger)`
  - `EnsureSetupLink(ctx) error` — password mode and no people only; revokes unused setup links, creates one, logs at WARN `setup: open <url> to create the owner`.
  - `NewSetupLink(ctx) (string, error)` — same without logging; `apperror.Conflict("this Kalzone already has an owner")` when people exist.
  - `NewResetLinkFor(ctx, login string) (string, error)` — for the subcommand; refuses disabled and GitHub users with `apperror.Conflict("reset links are only for active password accounts")`.
  - `Complete(ctx, token string, in SetupInput) (SetupResult, error)` (publishes `setup.completed`) with `SetupInput{ Username, Password, Confirm string; Bypass bool }` and `SetupResult{ Session entities.IssuedSession; AgentToken string; MCPCommand string }`.
  - `TokenIssuer` interface: `Create(ctx, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error)` (satisfied by `tokens.TokensUsecase`); the agent token draft is name `agent`, every scope, team `default`, 90 days, `Bypass: in.Bypass`, created with the owner's session actor after the transaction.

- [ ] **Step 1: Write the failing tests** with fakes: `Test_EnsureSetupLinkOnlyWithoutPeople`, `Test_EnsureSetupLinkRevokesTheEarlierOne`, `Test_GitHubModeMakesNoSetupLink`, `Test_CompleteCreatesAnAdminOwnerAndAgentToken` (owner `Admin == true`, token draft scopes = all, team `default`, `ExpiresInDays == 90`, `MCPCommand == "claude mcp add --transport http kalzone <base>/mcp --header \"Authorization: Bearer <token>\""`), `Test_CompleteRefusesMismatchedPasswords`, `Test_ResetLinkRefusesDisabledAndGitHubUsers`.
- [ ] **Step 2: Run** `go test ./app/identity/usecases/accounts/` — expect failure.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Create the first owner from a one-time setup link"`

### Task 4: Rules — password sign-in and the rate limiter

**Files:**
- Modify: `internal/ratelimit/ratelimit.go` (+ `ratelimit_test.go`)
- Create: `app/identity/usecases/accounts/signin.go`, `signin_test.go`
- Modify: `app/identity/usecases/signin/signin.go` (refuse disabled users; apply admins only here, as today) and its test

**Interfaces:**
- Produces in `ratelimit`: `func (l *Limiter) Blocked(key string) bool` (no token left, consumes nothing), `func (l *Limiter) Fail(key string)` (consumes one), `func (l *Limiter) Clear(key string)`.
- Produces `type PasswordSignIn`, `NewPasswordSignIn(cfg config.Config, mode Mode, users UserReader, sessions SessionCreator)`, `SignIn(ctx, username, password, ip string) (entities.IssuedSession, error)` returning `errWrongCredentials` (`apperror.New(apperror.CodeSignInFailed, "Wrong username or password.")`) or `errThrottled` (`CodeSignInFailed`, "Too many attempts, try again later."), and `ClearFailures(login string)` used by reset.
- Keys: `"ip:"+ip` and `"user:"+login+"|"+ip`, both `PerHour(10)`.
- A failed sign-in is logged at INFO with `username` and `ip` fields only; it is not an audit event.

- [ ] **Step 1: Write the failing tests**: `Test_BlockedDoesNotConsume`, `Test_SignInWithThePassword`, `Test_SignInNormalizesTheUsername`, `Test_UnknownWrongAndDisabledLookTheSame` (same error value; unknown user calls `VerifyPassword(DummyHash, …)`), `Test_EleventhFailureIsThrottledWithoutChecking`, `Test_OtherIPStillSignsIn`, `Test_GitHubModeRefusesPasswordSignIn`, and in `signin_test.go` `Test_GitHubSignInRefusesADisabledUser`.
- [ ] **Step 2: Run** `go test ./internal/ratelimit/ ./app/identity/usecases/...` — expect failure.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Sign people in with a password, throttled per IP and per username and IP"`

### Task 5: Rules — invites, resets and people

**Files:**
- Create: `app/identity/usecases/accounts/invites.go`, `invites_test.go`, `people.go`, `people_test.go`
- Modify: `app/authz/authz.go`, `app/authz/token.go` and tests

**Interfaces:**
- Produces in `authz`: `ManageInvites Action = "team.invites"` (team maintainer or admin; token scope `scope.TeamsAdmin`), `ManagePeople Action = "people.manage"` (admin only, never for tokens).
- Produces `type InvitesUsecase` with `Create(ctx, a, teamSlug string, role string) (Invite, error)` (`Invite{ Link entities.AccountLink; URL string }`, URL only on create), `List(ctx, a, teamSlug) ([]entities.AccountLink, error)`, `Revoke(ctx, a, teamSlug, id string) error`, `Accept(ctx, token, username, password, confirm string, signedIn bool) (entities.IssuedSession, error)` (signedIn → `apperror.Conflict("Sign out to accept this invite")`).
- Produces `type PeopleUsecase` with `Remove(ctx, a, login string) error` (refuses the last admin with `apperror.Conflict("keep at least one admin")`), `IssueReset(ctx, a, login string) (string, error)`, `UseReset(ctx, token, password, confirm string) error` (then `PasswordSignIn.ClearFailures`), `ChangePassword(ctx, a, current, next, confirm, keepSessionHash string) error`.
- Every write publishes the audit events of spec §4.6 through the existing `Publisher`, with no secret fields.

- [ ] **Step 1: Write the failing tests**: `Test_MaintainerInvitesToOwnTeamOnly`, `Test_InviteFixesTeamAndRole`, `Test_GitHubModeRefusesInvitesAndResets` (`CodeGitHubMode`), `Test_RemoveKeepsTheLastAdmin`, `Test_ResetLinkOnlyForActivePasswordUsers`, `Test_ChangePasswordNeedsTheCurrentOne`, `Test_AuditEventsCarryNoSecrets` (marshal every published event and assert it contains neither the password nor the token).
- [ ] **Step 2: Run** `go test ./app/authz/ ./app/identity/usecases/accounts/` — expect failure.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Invite, reset and remove people with password accounts"`

### Task 6: Handlers, pages, API, MCP and the subcommand

**Files:**
- Create: `app/identity/handlers/accounts/pages.go` (`GET/POST /join/:token`, `POST /login`), `app/identity/handlers/accounts/invites.go` (API), `app/web/templates/pages/join.html`, `app/web/templates/pages/setup_done.html`
- Modify: `app/web/handlers/login/login.go`, `app/web/templates/pages/login.html` (password mode: username/password form, text "Ask the owner of this Kalzone for an invite link", no GitHub button; GitHub mode: unchanged), `app/web/handlers/people/people.go`, `app/web/templates/pages/people.html`, `app/web/handlers/team/team.go`, `app/web/templates/pages/team.html`, the avatar partial in `app/web/templates/partials/components.html` (initials when `AvatarURL` is empty), `app/identity/handlers/auth/auth.go` (`/auth/github` and callback answer 404 in password mode)
- Modify: `internal/mcpserver/tools.go` (nothing to change in `excluded`: the three invite operations are new and stay included; `removePerson` is web-only)
- Modify: `cmd/controlplane/main.go`, `cmd/controlplane/modules/identity.go` (provide the new usecases, repositories and handlers; `fx.Invoke` a start hook that calls `EnsureSetupLink`)
- Modify: `README.md` quick start and `docs/development.md` (setup link, the two modes, switching modes is unsupported)

**Interfaces:**
- API operations (scope `scope.TeamsAdmin`): `createInvite` `POST /teams/:team/invites` body `{"role"}` → `201 {"id","role","expires_at","url"}`; `listInvites` `GET /teams/:team/invites` → page of `{"id","role","created_by","expires_at"}`; `revokeInvite` `DELETE /teams/:team/invites/:id` → 204.
- `/join/:token` renders the setup form, the invite form or the reset form by link kind; GitHub mode → 404.
- Subcommand: `controlplane setup-link [--reset <login>]` builds `fx.New(options.Options(), fx.NopLogger, fx.Invoke(printSetupLink))` without starting the app, prints the URL on stdout, exits 1 with the error message otherwise.

- [ ] **Step 1: Wire** handlers and routes; add `"GET /join/:token"`, `"POST /join/:token"`, `"POST /login"` to `publicRoutes` in `internal/integration/tests/auth_test.go`.
- [ ] **Step 2: Run** `go build ./... && go test ./internal/mcpserver/ ./app/...` — expect PASS (the contract test lists the three new operations).
- [ ] **Step 3: Commit** `git commit -m "Serve setup, sign-in, invite and reset pages, the invite API and setup-link"`

### Task 7: Integration tests

**Files:**
- Modify: `internal/integration/suite.go` (option `WithPasswordMode(cfg)` that empties `GitHub.ClientID/ClientSecret`; capture the logger output in `Suite.Logs`), `internal/integration/client.go` (`func (s *Suite) SignInWithPassword(t, login, password string) *Client`, `func (s *Suite) SetupLink(t) string` read from `Suite.Logs`)
- Create: `internal/integration/tests/accounts_test.go`

- [ ] **Step 1: Write the tests**, one per bullet of spec §5 plus Review Focus 3 and 4: `Test_SetupCreatesOwnerTeamAndWorkingToken` (token works on `/api/v1/whoami` and `/mcp`), `Test_SetupLinkWorksOnce`, `Test_NoSecondSetupLinkOnceOwned`, `Test_DeadLinksShowOnePage`, `Test_SignInFailuresLookTheSame`, `Test_EleventhFailureIsThrottled`, `Test_InviteJoinsTeamWithItsRole`, `Test_InviteRoleCannotBeEdited`, `Test_InviteAsksASignedInPersonToSignOut`, `Test_RemovedPersonIsSignedOutEverywhere` (their token → 401 on `/api/v1/whoami`, their session → redirect to `/login`), `Test_PasswordModeHidesGitHub` (`/auth/github` 404, no GitHub button), `Test_GitHubModeHidesPasswords` (`POST /login` and `/join/x` 404, no setup link in logs, `createInvite` → 409 `err_github_mode`), `Test_LocalLoginInAdminsListIsNotAdmin`, `Test_NoSecretsInLogsOrAudit` (after a full setup, invite, reset and sign-in flow, `Suite.Logs` and every `audit_events.details` contain no password, hash prefix `$argon2id$` or link token beyond the one setup line).
- [ ] **Step 2: Run** `make test` — expect PASS for the whole suite.
- [ ] **Step 3: Commit** `git commit -m "Test local accounts end to end"`

### Task 8: Shape the history

- [ ] **Step 1:** Use the `layered-commits` skill to rewrite the branch into storage (Tasks 1–2), rules (Tasks 3–5), handlers with wiring and integration tests (Tasks 6–7), keeping the design commit first.
- [ ] **Step 2: Run** `make lint && make test` on the rewritten branch — expect PASS.

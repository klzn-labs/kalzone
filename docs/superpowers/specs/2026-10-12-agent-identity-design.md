# Agent identity — design

Date: 2026-10-12
Status: approved in conversation, open for review
Sub-project 1 of 7 of "a platform made for AI agents".

## 1. Why

On 2026-10-12 an agent (Claude Code) onboarded a test service and shipped a change to
three Plateful services through the platform. Every API call needed a session that
the agent could only get by SSH-ing into the VPS, using the production MySQL root
password to insert a row into `sessions`, sending the cookie plus a CSRF header, and
deleting the row afterwards. The audit log recorded everything as the human user.
Earlier sessions (2026-10-07 to 2026-10-09) did the same.

The user's goal: the platform is made specifically for AI agents to interact with,
from three places, in this order:

- **A.** Claude Code on the user's machine.
- **B.** Agents in the cloud: scheduled routines, Claude on the web, CI.
- **C.** Agents running inside the platform.

Changes to protected environments (prod) are **requested** by agents and approved by
a human, unless the agent's token was explicitly given a bypass.

## 2. The seven sub-projects

| # | Sub-project | Depends on |
|---|---|---|
| **1** | **Agent identity** (this document): scoped tokens, team limits, bypass permission, Bearer auth, audit per token | — |
| 2 | Approvals for protected environments: pending requests a human approves or rejects | 1 |
| 3 | Agent-grade API: OpenAPI, one response and error shape with hints, `?wait=`, friendly references (`latest`, commit SHA), batch operations, spec merge-patch, compact events, config search | 1 |
| 4 | Remote MCP server at `/mcp` over the same usecases | 1, 3 |
| 5 | Diagnostics: runtime container status, logs through the API, classified deploy failures | 3 |
| 6 | Verifiable onboarding: a service check for workflow, GitHub App, registry secrets and image requirements; health checks that do not need `wget` in the image | 3 |
| 7 | OAuth so claude.ai connectors can use `/mcp` | 1, 4 |

## 3. Who an agent is

A token **belongs to a human user** and acts on their behalf, never above them:

> what a token may do = **owner's permissions** ∩ **token scopes** ∩ **token teams**

- The owner's current role is checked on every request: losing maintainer on a team
  takes prod on that team away from all the owner's tokens at once.
- A platform admin's tokens do **not** inherit "admin may do anything". They are
  bound by their scopes and teams like everyone else's.
- Agents inside the platform (scenario C) get a token owned by the human who set them
  up. The platform has no identity of its own.

Service accounts (principals with no human owner) are out of scope. They can be added
later with the same scopes.

## 4. Scopes

| Scope | Allows | Existing `authz` actions |
|---|---|---|
| `read` | Read services, config (never secret values), releases, deployments, queues, audit | reads |
| `services` | Create and edit services and queues. Queue edits on a protected environment follow the config rule below | `service.create`, `service.edit`, `queue.edit` |
| `services:admin` | Delete and transfer services | `service.delete`, `service.transfer` |
| `builds` | Request a build, register a release | `release.create` |
| `config` | Revise config (variables, spec) | `config.edit` |
| `secrets` | Set and unset secrets | `config.edit` on secret routes |
| `deploy` | Deploy, promote, roll back, apply config | `deploy` |
| `logs` | Read container logs (endpoints arrive in sub-project 5). Not implied by `read`, because logs can leak secrets and personal data | — |
| `protected` | Extends `config`, `secrets` and `deploy` to protected environments | — |
| `teams:admin` | Manage teams and members | `team.members`, `teams.manage` |
| `tokens:admin` | Create and revoke tokens for the same owner | new: `tokens.manage` |

`admins.manage` is never available to a token.

Separately from scopes, a token may carry the **bypass** permission.

Protected environments are identified by the existing `environments.protected` flag,
not by name, so a future `staging` needs no change.

| Token on a protected environment | Result |
|---|---|
| no `protected` | 403 `err_missing_scope` |
| `protected`, no bypass | 403 `err_approval_required` until sub-project 2, then a pending request |
| `protected` and bypass | runs immediately, audited as a bypass |

Reads are limited by the token's teams as well: resources owned by other teams are
left out of lists and return 404 when asked for directly.

## 5. Token lifecycle

**Format.** `plat_` followed by 32 random bytes in base62. The prefix makes leaked
tokens easy to find. Only the SHA-256 hash is stored; the token is shown once.

**Stored per token.** Id, owner, name (unique per owner among tokens that are not revoked; revoking frees the name, expiring does not), hash, display prefix (first
8 characters after `plat_`), scopes, team ids, bypass, created at, created by (the
session or token that made it), expires at, last used at and IP, revoked at.

**Teams.** An explicit, non-empty list. There is no "all my teams", so a token never
gains a team its owner joins later.

**Expiry.** Required. Default 90 days, maximum 1 year. Revocation takes effect on the
next request, because every request looks the token up, as sessions already do.

**Who creates what.**

- Any signed-in user creates tokens for themselves in the UI.
- A token with `tokens:admin` creates tokens for **its own owner** whose scopes and
  teams are subsets of its own (no escalation). A child never outlives its parent:
  a longer expiry is shortened to the parent's.
- **Bypass is only granted by a platform admin in a browser session.** No token can
  issue a token with bypass, even one that has bypass.
- Admins list and revoke every user's tokens.

## 6. Authentication

- `Authorization: Bearer plat_…` is accepted on `/api/v1` (and on `/mcp` when it
  exists).
- With a Bearer token there is no CSRF check. CSRF guards cookies the browser sends
  on its own, and a browser never sends a Bearer header on its own.
- When a Bearer header is present the session cookie is ignored, so a request always
  has exactly one actor.
- UI pages (htmx) reject Bearer tokens.
- 401 codes: `err_token_invalid`, `err_token_expired`, `err_token_revoked`.

`actor.Actor` gains a `Token` field (id, name, scopes, team ids, bypass), empty for
browser sessions. `authz.Policy.Can` first applies today's rules to the owner, then
requires the action's scope and the resource's team on the token, then the
protected-environment rule from section 4.

## 7. API and UI

- `GET /api/v1/whoami`: owner, token (name, prefix, scopes, teams, bypass, expiry).
  The first call an agent makes to learn what it may do.
- `GET /api/v1/tokens`, `POST /api/v1/tokens`, `DELETE /api/v1/tokens/:id`. From a
  token these need `tokens:admin`.
- UI page "Access tokens" in the user menu: list with last use, create (name, scopes,
  teams, expiry; bypass only for admins), revoke. Admins also see everyone's tokens.

## 8. Errors that teach

- Missing scope: `403 {"code":"err_missing_scope","message":"this token lacks deploy","required_scope":"deploy"}`.
- Team not on the token: `403 err_team_not_granted`, naming the team.
- The owner lacks the role: today's `403 err_forbidden`, so "the token may not" and
  "its owner may not" stay distinguishable.

## 9. Audit

`audit_events` gains `token_id` and `token_name` (empty for browser sessions). The UI
shows "adrianozp via `claude-local`" and filters by token. Token created, token
revoked and every bypass use are audit events of their own.

## 10. Storage

Migration `000005_access_tokens`:

- table `access_tokens` with the fields of section 5, a stored generated column
  holding the name while the token is not revoked and unique `(owner_id, that column)`,
  unique `token_hash`, index on `owner_id`;
- `audit_events.token_id CHAR(36) NOT NULL DEFAULT ''`,
  `audit_events.token_name VARCHAR(64) NOT NULL DEFAULT ''`, index on `token_id`.

## 11. Testing

- Unit, `authz.Policy`: the table of owner role × scopes × teams × protected × bypass;
  admin owner not inherited; `admins.manage` never allowed.
- Unit, token usecases: no escalation on create, bypass never issued by a token,
  expiry bounds, a child's expiry capped at its parent's, name uniqueness and reuse
  after revoking.
- Integration: Bearer on `/api/v1` passes without CSRF; Bearer on a UI page is
  rejected; expired and revoked tokens get their 401 codes; Bearer wins over a
  cookie; the audit row carries the token; reads outside the token's teams are
  filtered.

## 12. Done when

A token created in the UI lets an agent deploy `hello` to dev with no SSH and no SQL,
the audit shows "adrianozp via <token>", and the same token gets
`err_approval_required` on prod.

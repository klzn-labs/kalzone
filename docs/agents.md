# Agents: tokens, API, MCP and approvals

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
outlives it. Tokens expire after 1 to 365 days (90 by default) and are shown
once; only a hash is stored.

On a protected environment (`prod`) a token needs `protected`, and then its change
waits for approval (see [Approvals](#approvals)) unless a platform admin gave it bypass from the website.
Tokens are refused on website pages and need no CSRF header. Every change made
with a token shows in the audit log as "person via token".

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/whoami` | owner, and for a token its scopes, team slugs, bypass and expiry (a browser session gets the owner only) |
| `GET /api/v1/tokens` | your tokens with their team slugs (`?all=true` for admins in the browser) |
| `POST /api/v1/tokens` | `{"name","scopes","teams","expires_in_days","bypass"}` → the token, once |
| `DELETE /api/v1/tokens/:id` | revoke |

## API for agents

`GET /api/v1/openapi.json` describes every operation: its `operationId`, the token
scope it needs, path and query parameters, the request body, every response, and the
values of every status field. Any valid token or session reads it. It is generated
from the handlers' declarations, so it is always the API that runs.

- A single resource is the bare object; actions that start work (build, deploy,
  promote, config apply, rollback) answer `202` with it.
- A list is `{"items": [...], "next_cursor": "..."}`: `?limit` 1-100 (20 by
  default), and `?cursor=<next_cursor>` for the next page; no `next_cursor` means
  the end.
- An error is `{"error": {"code", "message", "hint", "field", "details"}}`; `hint`
  says how to fix the call and `field` names the offending field or parameter.
  `hint`, `field` and `details` appear only when they apply.
- Bodies are strict: an unknown field is a `400` that lists the accepted fields.
- `GET /api/v1/deployments/:id` carries `events_summary` (`total` lines,
  `progress_lines` counting the docker layer progress lines the tail skips, `phases` with the seconds spent waiting and running, the
  last 20 other lines as `tail`, and the `log` path); the full log is
  `GET /api/v1/deployments/:id/events`, oldest first. `?tail=N` (1-100) answers the
  last N lines instead and cannot be combined with `cursor`. While a deployment
  runs, `next_cursor` stays so the log can be followed.
- `?wait=30s` (above 0, at most `120s`) on `GET` of a release, deployment or change
  request, and on the calls that start a build, deploy, promotion, config apply or
  rollback, answers when the status is final with `200`, or when the wait ends with
  `202`, the current state and `Retry-After: 5`. A call that became a change request
  waits for the request instead.
- A release is named by its id or, within a service, by `latest` (the last built),
  `r42`, `sha:<7 to 40 hex>` (the newest build of that commit; a prefix shared by two
  commits is refused) or a tag: in `release` of `POST /api/v1/deployments`, and in
  `GET /api/v1/releases/<ref>?service=<slug>`.
- `POST /api/v1/deployments` and `POST /api/v1/deployments/promotions` take
  `targets` (1 to 10 `{"service", "environment"}`) instead of one service: every
  target is checked first, then all are deployed in one transaction, or none is. The
  answer is `{"items": [...]}` in target order. A batch with a target that needs
  approval is refused; send those targets one at a time.
- `PATCH /api/v1/config/:service/:env` with `Content-Type: application/merge-patch+json`
  writes a revision that changes only what the patch names in `spec` and `env`;
  `null` removes a field or a variable, and `spec: null` or `env: null` is a `400`.
  Secrets keep their own endpoints.
- `GET /api/v1/config/search?key=LOG_LEVEL` lists where a variable or secret is set in
  the current configs of the services the caller sees, narrowed by `env` and `team`;
  the key is matched exactly and secrets are listed by name only.

## MCP for agents

`/mcp` is a Model Context Protocol server over the same API (Streamable HTTP,
stateless, JSON answers). It takes the same access tokens, as a bearer only:

```
claude mcp add --transport http kalzone https://platform.kalz.one/mcp \
  --header "Authorization: Bearer plat_…"
```

- Every operation of `openapi.json` is a tool named by its `operationId`
  (`createDeployment`, `patchConfig`), with path, query and body fields as one input
  object; a body whose field shares a parameter's name (`patchConfig`'s `env`) goes
  whole under `body`. A call runs as the API call it names, with the same checks,
  approvals, audit and error hints; an error comes back as a tool error with the
  error body.
- `tools/list` shows only the tools the token's scopes allow.
- Not tools: tokens, users, team administration, deleting or transferring a service,
  deleting a queue, unsetting a secret, `getMe` and the document itself.
- Tools that start a build or a deployment take `wait: true` to answer once the
  result is final (up to 120s); the `wait` tool waits on a release, a deployment or a
  change request by `kind` and `id`, for up to `seconds` (120 by default). An answer
  that is not final yet says so.
- On a protected environment a change answers a change request with its
  `approve_url`; `wait` with `kind: request` follows it.
- `purgeDeadLetters` needs `confirm` set to the queue's name.

## Approvals

A token with `protected` but without bypass does not change a protected environment
directly. Config revisions, secrets, deploys, promotions, rollbacks, config
applications and queue create, update and delete answer `202`:

```json
{"kind": "change_request", "id": "…", "status": "pending", "summary": "deploy release #14 to prod",
 "approve_url": "https://platform.kalz.one/requests/…", "poll": "/api/v1/requests/…", "expires_at": "…"}
```

A maintainer of the service's team or an admin, the token's owner included, opens
**Requests** in the browser, sees the change against what runs now, and approves or
rejects it. Approval runs the change as the token, re-applied on the current state; it
fails with `err_stale` for config and deploy changes only when the same config key
changed or another deployment went live in the meantime, and a queue update fails with
`err_conflict` when it finds the queue changed since the request. A newer request for the same thing cancels the older one,
and requests expire after 24 hours. Promote pins the release that was live when asked.
Queue pause, resume and dead-letter commands still answer `err_approval_required`.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/requests/:id` | `status` (`pending`, `approved`, `executed`, `failed`, `rejected`, `cancelled`, `expired`), `result`, `reason`, `error` |
| `GET /api/v1/requests?status=pending` | the token's teams' requests, newest first, paged |
| `POST /api/v1/requests/:id/cancel` | withdraw your own request |

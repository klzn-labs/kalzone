# MCP server — design

Date: 2026-10-14
Status: approved in conversation (the user told the agent on 2026-10-14 to follow the recommended option at every decision), open for review
Sub-project 4 of 7 of "a platform made for AI agents". Builds on sub-project 1
(identity), sub-project 2 (approvals) and sub-project 3 (agent-grade API), all live.

## 1. Why

Since sub-project 3 an agent can drive the platform through `/api/v1`, but it has to
read `openapi.json`, build URLs and send `curl` calls by itself. Agent hosts (Claude
Code, Claude on the web, other MCP clients) speak the Model Context Protocol natively:
a remote MCP server gives them typed tools, so an agent builds, deploys and requests
a prod change without one hand-written HTTP call.

**Done when** Claude Code on the user's machine is configured with

```
claude mcp add --transport http platform https://platform.kalz.one/mcp \
  --header "Authorization: Bearer plat_…"
```

and, with the `claude-local` token, lists only the tools that token's scopes allow,
builds `hello`, deploys it to `dev` and waits for the result, and gets a change
request back from a deploy to `prod`, all through tools.

## 2. Decisions

The user's decisions of 2026-10-12 and 2026-10-14, binding:

| # | Topic | Decision |
|---|---|---|
| 1 | SDK and transport | The official `github.com/modelcontextprotocol/go-sdk` (v1.6.1), Streamable HTTP, **stateless** (no `Mcp-Session-Id`, no server-to-client stream), JSON responses. |
| 2 | Where | `/mcp` in the control plane, next to `/api/v1`, same process and port. |
| 3 | Auth | `Authorization: Bearer plat_…` only, the same tokens as `/api/v1`. A browser session cookie is not accepted. OAuth is sub-project 7. |
| 4 | Granularity | **One tool per catalog operation**, named by its `operationId` (`createDeployment`), plus one `wait` tool. No hand-made grouping. |
| 5 | Execution | A tool call is **dispatched in process through the same gin engine** as an `/api/v1` request with the caller's `Authorization` header. Guard, authz, strict binding, approvals, `?wait`, audit and error hints are those of the API by construction. |
| 6 | Input schemas | Derived from the generated OpenAPI document (the one served at `/api/v1/openapi.json`), not reflected a second time. |
| 7 | Visibility | `tools/list` shows a tool only when the token has the operation's scope (or the operation needs none); a hidden tool is not registered, so calling it by name answers "unknown tool". Enforcement stays in the API either way. |
| 8 | Out of v1 | Deleting and transferring, teams administration, tokens, users and the document itself (§5). |
| 9 | Waiting | Every waiting operation's tool takes `wait: true` (wait up to 120 s); the `wait` tool waits on a release, deployment or change request by id. |
| 10 | Purge | `purgeDeadLetters` needs `confirm` equal to the queue's name. |

## 3. Architecture

```
MCP client ──POST /mcp──▶ token guard ──▶ mcpserver.Handler
                                            │ per request: mcp.Server with the tools the token sees
                                            ▼
                                   tool call ──▶ dispatch: http.Request /api/v1/… (same Authorization)
                                                    │
                                                    ▼
                                              gin engine (api guard, authz, handlers)
                                                    │
                                   tool result ◀────┘ status + JSON body
```

New package `internal/mcpserver`, infrastructure like `internal/apispec`:

| File | Job |
|---|---|
| `tools.go` | `Tools`: build every tool definition once, at start, from the catalog and the OpenAPI document; the exclusion list; visibility by scope |
| `schema.go` | turn one OpenAPI operation into a tool input schema |
| `dispatch.go` | turn tool arguments into an `/api/v1` request, run it through the engine, turn the answer into a tool result |
| `server.go` | the per-request `mcp.Server`: implementation name, instructions, visible tools, the `wait` tool |
| `routes.go` | `RegisterMCPRoutes`: start hook that builds the tools, `POST`, `GET` and `DELETE /mcp` on the public router behind the token guard |

In stateless mode the SDK's handler answers `GET /mcp` with `405` (no stream to
open) and `DELETE /mcp` with `204` (no session to end).

**Engine re-entry.** The handler holds the `*gin.Engine` (an `http.Handler`) and calls
`ServeHTTP` with an in-memory recorder. The engine routes `/api/v1/…` exactly as for a
network request. The dispatched request copies from the MCP request: `Authorization`,
`RemoteAddr`, `X-Forwarded-For`, `X-Real-IP`, `User-Agent` and the context (cancelling
the MCP call cancels the API call and its wait).

**Token guard.** `httpserver.Guards` gets a fourth guard, `Token`, built in
`app/identity/handlers/guard`: it authenticates the bearer token and sets the viewer,
nothing else (no read check: tools enforce their own). Without a bearer it answers
`401` with `WWW-Authenticate: Bearer` and the usual error body, hint "send
Authorization: Bearer <token>, a token from the Tokens page". The pages' error text
"access tokens work on /api/v1 only" becomes "on /api/v1 and /mcp only".

The token is authenticated twice per tool call (at `/mcp` and in the dispatched
request). Both are the same cheap indexed lookup; keeping the dispatched request a
plain API request is worth it.

## 4. Tool definitions

Built once in a start hook (after every `Register*Routes` invoke filled the catalog)
from `apispec.Build(catalog, baseURL)`. A definition holds the `mcp.Tool` and what the
dispatcher needs: method, path, path parameter names, query parameter names, body
mode and body content type.

**Name and description.** Name = `operationId`. Description = the summary, then the
operation's notes (gated, waiting) as the document writes them, then "Needs scope
`<scope>`." when it has one.

**Annotations.** `GET` → `readOnlyHint`. `PUT` → `idempotentHint`. `DELETE` →
`destructiveHint: true`. Every tool → `openWorldHint: false`.

**Input schema**, an object:

- Each path and query parameter becomes a property with its name and schema;
  `required` follows the parameter. The query parameter `wait` becomes
  `wait: boolean` ("true waits up to 120s for a final status").
- The JSON body: when its schema (after following one `$ref`) is an object with
  properties and none of them has a parameter's name, its properties and `required`
  are merged into the input. Otherwise the whole body becomes one required property
  `body` with that schema (`patchConfig`: the path's `env` and the patch's `env`).
- `$ref`s to `#/components/schemas/X` become `#/$defs/X`, and every schema reached is
  copied into the input's `$defs`.
- `additionalProperties: false`.
- Two parameters with the same name fail the start: the test catches it before a
  deploy.

**Fix in `apispec`.** The merge-patch body of `patchConfig` is documented today as
`{"type": "string"}`, because swaggest only reflects bodies of known content types.
`apispec.request` reflects the body as JSON and then moves the media type to the
operation's `BodyType`, so the document shows the real `patchDTO` object under
`application/merge-patch+json`. The contract test already validates the patch calls.

## 5. Tools in v1

Every catalog operation is either a tool or on the exclusion list in `tools.go`; a
test fails on an operation that is neither, so a new endpoint needs a decision.

Excluded: `getOpenAPI`, `getMe`, `listTokens`, `createToken`, `revokeToken`,
`createTeam`, `deleteTeam`, `putMember`, `removeMember`, `listUsers`, `getUser`,
`setAdmin`, `deleteService`, `transferService`, `deleteQueue`, `unsetSecret`.

Tools (43 + `wait`): `getWhoami`; `listServices`, `getService`, `createService`,
`updateService`; `listTeams`, `getTeam`, `listMembers`; `listEnvironments`;
`listReleases`, `getRelease`, `requestRelease`, `registerRelease`; `listDeployments`,
`getDeployment`, `listDeploymentEvents`, `createDeployment`, `promoteDeployment`,
`applyConfig`, `rollbackDeployment`; `getConfig`, `patchConfig`, `reviseConfig`,
`listRevisions`, `listSecrets`, `setSecret`, `searchConfig`; `listQueues`, `getQueue`,
`createQueue`, `updateQueue`, `pauseQueue`, `resumeQueue`, `getQueueStats`,
`listQueueVersions`, `listDeadLetters`, `replayDeadLetter`, `replayDeadLetters`,
`purgeDeadLetters`; `listRequests`, `getRequest`, `cancelRequest`; `listAuditEvents`.

**`wait` tool.** Input `{kind: "release" | "deployment" | "request", id, seconds}`,
`seconds` 1–120, default 120. Dispatches `getRelease`, `getDeployment` or
`getRequest` with `?wait=<seconds>s`. Scope `read`.

**`purgeDeadLetters`** gets a required `confirm` property ("the queue's name again").
When it differs from `name` the tool answers an error result, hint "confirm must be
the queue's name", without calling the API; otherwise `confirm` is dropped and the
call dispatched.

## 6. Dispatch and results

**Request.** The path is the operation's path with each `:param` replaced by the
argument, `url.PathEscape`d. Query arguments are formatted with `fmt.Sprint` (strings,
numbers, booleans); `wait: true` becomes `wait=120s`, `wait: false` is dropped. Body
fields go into a JSON object, or the `body` argument is sent as is; content type is
the operation's `BodyType` or `application/json`. A missing path argument answers an
error result naming it.

**Result.**

| API answer | Tool result |
|---|---|
| 2xx with a JSON body | text content = the body (no `structuredContent`: clients would pass the same JSON to the model twice) |
| `202` with `Retry-After` (a wait that ended first) | the same, plus a second text: "Not final yet: call the wait tool with the id in this answer." |
| `204` | text "Done." |
| 4xx, 5xx | `isError: true`, text content = the error body (`error.code`, `message`, `hint`, `field`) |

A change request (`202` with `approval.Pending`) is a successful result: its body
already carries `status: pending`, `approve_url` and the hint to wait on it.

**Server instructions** (`InitializeResult.instructions`), one paragraph: what the
platform is; every tool acts as the token's owner within its scopes and teams; on a
protected environment a change answers a change request that a human approves at
`approve_url`, and `wait` with kind `request` follows it; long operations take
`wait: true`; errors carry a hint that says how to fix the call.

## 7. Errors and limits

- Tool arguments that do not match the schema reach the API and fail its strict
  binding with a field and a hint; the MCP layer does not validate a second time.
- A dispatched call inherits the API's limits (`?wait` ≤ 120 s, batch ≤ 10).
- An unknown tool name answers the SDK's "unknown tool" error.
- An argument that is neither a parameter nor a body field answers an error result
  naming it, instead of being dropped.
- `tools/list` and `initialize` need only a valid token; a token without `read` sees
  `getWhoami` and the tools of its other scopes.

## 8. Testing

**Unit** (`internal/mcpserver`):

- schema: parameters and body merged; `required` kept; `wait` becomes a boolean;
  a non-object body becomes `body`; `$ref`s rewritten and `$defs` collected
  transitively; a name collision fails.
- dispatch: path escaping, query formatting, `wait: true`, body modes, copied
  headers; result mapping for 200, 202 with `Retry-After`, 204 and an error body.
- purge confirmation; `wait` tool argument checks.
- visibility by scope.

**Integration** (`internal/integration/tests/mcp_test.go`, the go-sdk client over
`StreamableClientTransport` against the suite's server):

- every catalog operation is a tool or excluded; every input schema resolves with
  the SDK's JSON Schema package.
- a read-only token lists no `createDeployment`; a deploy token does.
- the flow: `registerRelease` → `createDeployment` to `dev` with `wait: true` while
  the agent runs it → `succeeded`; `createDeployment` to `prod` → change request result.
- an API error comes back as `isError` with the hint.
- `purgeDeadLetters` with a wrong `confirm` changes nothing.
- no bearer → `401` with `WWW-Authenticate: Bearer`; a session cookie alone → `401`.
- the audit event of a tool call names the token.
- the `apispec` fix: `patchConfig`'s body in the document is an object.

## 9. Delivery

Branch `mcp`, one commit per layer: the `apispec` body fix; `internal/mcpserver`
with its unit tests; the guard, wiring and integration tests; README ("MCP for
agents": the `claude mcp add` line, visibility, waiting, approvals). PR, merge,
blue-green release, then the done-when check of §1 from this machine.

## 10. Later

OAuth for claude.ai connectors (sub-project 7) adds the protected-resource metadata
to the `401` (`resource_metadata` in `WWW-Authenticate`). Diagnostics tools arrive
with sub-project 5. Progress notifications during waits and resources (the OpenAPI
document as a resource) are not needed by v1's clients.

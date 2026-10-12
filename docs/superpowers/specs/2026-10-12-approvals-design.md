# Approvals for protected environments — design

Date: 2026-10-12
Status: approved in conversation (user accepted the recommended decisions on 2026-10-12), open for review
Sub-project 2 of 7 of "a platform made for AI agents". Builds on sub-project 1,
`docs/superpowers/specs/2026-10-12-agent-identity-design.md`, which is live.

## 1. Why

Since sub-project 1 an agent holds a token. On a protected environment (`prod`) a
token with the `protected` scope but no bypass gets `403 err_approval_required` for
config, secrets, queue edits and deploys. The agent can do nothing with that answer
except tell a human to repeat the change by hand.

After this sub-project the same call answers `202` with a **change request**: the
platform keeps the validated change, tells a human, and runs it when the human
approves. The agent learns the outcome by polling.

## 2. Decisions

The user's decisions of 2026-10-12, binding:

| # | Topic | Decision |
|---|---|---|
| 1 | Who approves | A maintainer of the service's owning team or a platform admin, **including the token's owner**, only in a browser session, never through a token. "Approver ≠ owner" can be added later. |
| 2 | Notification | A counter in the UI navigation, an `approve_url` in the agent's answer, and a Discord message sent by the control plane to `PLATFORM_APPROVALS_WEBHOOK_URL`. Delivery is asynchronous and its failure never fails the request. |
| 3 | State moved before approval | Re-apply the change on the current state and fail only on a real conflict. The approval page diffs against the **current** state and warns when the base moved. |
| 4 | Secrets | The value is encrypted when the request is created (a `secret_versions` row); approvers only ever see the name. |
| 5 | Several pending requests | A new request supersedes the pending ones for the same thing (cancelled, reason "superseded by …"), like `clearWay` does for pending deployments. |
| 6 | Expiry | 24 hours, applied by the existing one-minute sweeper. |

Resolved in this document (section 16 lists the ones worth revisiting):

- Supersede key is the **subject** of the change, not only its kind (section 8).
- Queue pause/resume and dead-letter replay/purge do **not** become requests.
- Requests are created only by tokens; a browser session never needs approval.
- Polling needs the `read` scope like every other `GET`.

## 3. What becomes a request

A request is created only when the token passed every other check (owner's role,
scope, `protected` scope, team) and only the approval is missing. Every other refusal
keeps its 4xx answer.

| API call on a protected environment | Kind | Subject (supersede key) | Pinned in the request |
|---|---|---|---|
| `POST /config/:service/:env/revisions` | `config.revise` | `config` | the change set (spec, set/unset env), base revision |
| `PUT /config/:service/:env/secrets/:name` | `secret.set` | `secret.NAME` | the id of the encrypted secret version, base revision |
| `DELETE /config/:service/:env/secrets/:name` | `secret.unset` | `secret.NAME` | the name, base revision |
| `POST /deployments` | `deploy` | `deploy` | release id, strategy, live deployment id |
| `POST /deployments/promotions` | `promote` | `deploy` | **the release live in the source environment at request time**, live deployment id in the target |
| `POST /deployments/:id/rollback` | `rollback` | `deploy` | the target deployment id, live deployment id |
| `POST /deployments/config-applications` | `config.apply` | `deploy` | live deployment id |
| `POST /queues` | `queue.create` | `queue.NAME` | the queue draft |
| `PATCH /queues/:env/:name` | `queue.update` | `queue.NAME` | the change, including its base version |
| `DELETE /queues/:env/:name` | `queue.delete` | `queue.NAME` | the `force` flag |

Queue pause/resume and dead-letter replay/purge on a protected environment keep
answering `403 err_approval_required`: they are operational commands during an
incident, where waiting for an approval is the wrong tool. Bypass tokens and humans
in the browser still run them.

Promote is the only action whose meaning changes over time ("whatever dev runs now"),
so the request stores the release id it resolved, and approval deploys exactly that
release.

## 4. Lifecycle

```
pending ──approve──▶ approved ──run──▶ executed
   │                    │
   │                    └──────────▶ failed   (conflict, validation, token gone, crash)
   ├──reject──▶ rejected
   ├──cancel──▶ cancelled   (by the person who asked, or "superseded by <id>")
   └──24 h────▶ expired
```

| Status | Final | Set by |
|---|---|---|
| `pending` | no | the agent's call |
| `approved` | no | an approver; the change runs in the same HTTP request right after |
| `executed` | yes | the run succeeded; `result` names what it made ("config revision #13", "deployment #4 (id)", "queue version 2") |
| `failed` | yes | the run failed; `error_code` and `error_message` say why |
| `rejected` | yes | an approver, with an optional reason |
| `cancelled` | yes | the person who asked, or a newer request (reason `superseded by <id>`) |
| `expired` | yes | the sweeper, 24 hours after creation |

Every transition is a conditional `UPDATE … WHERE id = ? AND status = ?`, so two
approvers clicking at once produce one execution; the second gets
`409 "this request was already decided"`. A request still `approved` ten minutes
after its decision means the control plane stopped mid-run: the sweeper marks it
`failed` with "the platform stopped while running this request, check the target
before asking again".

## 5. Data model

Migration `000006_change_requests`, one table:

| Column | Type | Notes |
|---|---|---|
| `id` | `CHAR(36)` PK | |
| `kind` | `VARCHAR(32)` | section 3 |
| `subject` | `VARCHAR(160)` | supersede key |
| `service_id`, `service_slug` | `CHAR(36)`, `VARCHAR(40)` | the service as it was when asked; for queues the consumer |
| `team_id` | `CHAR(36)` | owning team when asked; decides who approves and who sees it |
| `environment` | `VARCHAR(32)` | |
| `summary` | `VARCHAR(255)` | one imperative line, e.g. `deploy release #14 to prod`, `change env.A, spec.replicas`, `set secret.DB_PASSWORD`; never a value of a secret |
| `payload` | `JSON` | the domain's pinned proposal (below) |
| `requested_by`, `requester_login` | | the token's owner |
| `token_id`, `token_name` | | the token that asked |
| `status` | `VARCHAR(16)` | section 4 |
| `decided_by`, `decider_login`, `decided_at` | | approver, rejecter, canceller, or `platform` for expiry |
| `reason` | `VARCHAR(500)` | rejection reason, `cancelled by <login>`, `superseded by <id>`, expiry note |
| `result` | `VARCHAR(255)` | after `executed` |
| `error_code`, `error_message` | | after `failed` |
| `created_at`, `expires_at`, `finished_at` | `DATETIME(3)` | |

Indexes: `(status, expires_at)` for the sweeper and the counter,
`(service_id, environment, subject, status)` for supersede, `(created_at)` for lists.

The payload belongs to the domain that made it; approvals stores and returns it
without reading it:

| Domain | Proposal |
|---|---|
| envconfig | `{kind, service, environment, base, change}`, where `change` is the existing `ChangeSet` extended with `set_secrets` (name → secret version id) and `unset_secrets` |
| deploy | `{kind, service, environment, release_id, strategy, from, deployment_id, live_id}` |
| queue | `{kind, environment, name, draft, change, force}` |

A `secret.set` request that is rejected, cancelled or expires leaves its encrypted
secret version unreferenced, the same as an overwritten secret today.

## 6. From an API call to a request

`authz.Policy` gains `Gate(ctx, actor, action, resource) (Gate, error)`, which runs
the existing rules and returns one of:

| Gate | When |
|---|---|
| `open` | browser sessions, unprotected environments, actions the protected rule does not cover |
| `bypassed` | a token with bypass on a protected environment |
| `approved` | a token running a change a human approved (the actor carries the approval, section 7) |
| `needs_approval` | a token with `protected` and without bypass on a protected environment |

`Can` stays for every other caller and maps `needs_approval` to the existing
`err_approval_required`. `authz.Direct(Gate(...))` does the same for callers that
must not create requests (queue pause and dead letters).

The config, deploy and queue usecases call `Gate` instead of `Can`, then validate
exactly as they do before writing: config validation, host and internal-service
rules and the no-op check; release built and owned by the service; nothing live for
apply/promote; config already live; queue settings and base version; queue not empty
on delete without force. If the gate says `needs_approval`, they hand a validated
proposal to a `Requests` port instead of writing:

```go
type Requests interface {
	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
}
```

`approval.Change` (in `pkg/approval`, shared like `pkg/events`) carries kind, subject,
service, team, environment, summary and the proposal. Submit stores the request,
supersedes older ones, publishes `request.created`, notifies, and returns
`*approval.Pending`. The usecase returns that value **as its error**: the change did
not happen, and every existing handler already sends errors to `httperror.Abort`,
which now answers `202` with the pending body when it finds one. No handler changes.

Secrets are sealed and their version row written before `Submit`, as decision 4
requires.

## 7. Approval and execution

**Who.** A new action `request.approve` requires the maintainer role on the request's
team; admins pass; it is not in any scope, so every token gets
`403 "no token can do this, sign in to the website"`. The approve and reject routes
live only on the web UI, behind the session guard, which refuses Bearer tokens.

**Run as whom.** On approval the platform rebuilds the token's actor from the stored
`token_id`: it must not be revoked or expired, and its owner is read again. The actor
gets `Approval{RequestID, ApproverID, ApproverLogin}`, and the domain usecase runs
through the same path as a direct call: owner role, scope, teams and the `protected`
scope are all checked again, and the gate answers `approved`. A revoked token, an
owner who lost the role or a removed scope ends as `failed` with that error's code.

**Re-validation and conflicts.** The change is re-applied on the current state, fully
validated again, and fails only on a real conflict:

| Kind | Conflict (`409 err_stale`, request `failed`) |
|---|---|
| `config.revise`, `secret.*` | a key the request changes was changed between its base revision and now. Keys are `env.NAME`, `secret.NAME`, and `spec` as one unit, because a change set replaces the whole spec. Other keys that moved are kept. |
| `deploy`, `promote`, `rollback`, `config.apply` | the live deployment is no longer the one live at request time |
| `queue.update` | the queue's version moved (the existing check, code stays `err_conflict`) |
| `queue.create`, `queue.delete` | none beyond normal validation (name taken, queue gone, not empty) |

Deploy, promote and config apply use the config revision current at approval.

**Outcome.** The approver's browser is redirected to the request page with a flash
naming the result or the error. The agent sees the same on its next poll.

## 8. Reject, cancel, supersede, expire

- **Reject**: approvers only, while pending and not past `expires_at`; optional
  reason up to 500 characters, shown to the agent.
- **Cancel**: only the person who asked (`requested_by`), from the browser or any of
  their tokens that sees the team; `POST /api/v1/requests/:id/cancel`. Approvers who
  do not want a change reject it.
- **Supersede**: after a request is stored, every other pending request with the same
  service, environment and subject is cancelled with reason `superseded by <new id>`.
  Subjects (section 3) make "deploy" and "promote" supersede each other, as two
  pending deployments do today, while two different secrets do not.
- **Expire**: the sweeper marks pending requests past `expires_at` as `expired`
  (decider `platform`). An approve or reject that arrives after `expires_at` but
  before the sweep is refused with `409 "this request expired, ask for a new one"`.

## 9. Agent API

The call that needs approval answers `202`:

```json
{
  "kind": "change_request",
  "id": "6f1c…",
  "status": "pending",
  "summary": "deploy release #14 to prod",
  "approve_url": "https://platform.kalz.one/requests/6f1c…",
  "poll": "/api/v1/requests/6f1c…",
  "expires_at": "2026-10-13T09:00:00Z"
}
```

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/requests/:id` | status, summary, who asked and decided, `reason`, `result`, `error {code, message}`, `approve_url`. 404 outside the token's teams. |
| `GET /api/v1/requests?status=pending` | newest 100, limited to the token's teams; `status` optional |
| `POST /api/v1/requests/:id/cancel` | only the person who asked |

Reads need the `read` scope like every `GET`. There is no approve endpoint on the API.

## 10. UI

- **Requests** in the sidebar (shield icon) with a counter of pending requests the
  viewer may decide. The counter loads by htmx from `GET /requests/badge` on page
  load and every 30 seconds, so pages do not need to compute it.
- `/requests`: "Waiting" and "Recent" tables (change, service, environment,
  "@owner via token", age, status badge).
- `/requests/:id`: summary, who asked with which token, expiry; while pending a
  table **Key / Now / After approval** computed against the current state (secret
  rows say `new value` or `removed`, never a value), a yellow notice when the base
  moved ("the config moved from revision 4 to 6 since the request") and a red one
  when approving will fail. Approve (with confirm) and Reject (optional reason) for
  those who may decide; "Cancel my request" for the person who asked. Decided
  requests show result, error or reason.
- Audit log lines for the request events link to the request page.
- Per the user's standing preference for visual changes, the implementer shows a
  mockup of the two pages before writing the templates; the plan's templates are the
  default option and follow the existing page style.

## 11. Discord notification

- New setting `approvals.webhook_url`, env `PLATFORM_APPROVALS_WEBHOOK_URL`; empty
  disables notifications.
- On `request.created` the control plane posts
  `{"content": "<login> via <token> asks to <summary> on <service> in <env>. Approve or reject within 24 hours: <approve_url>", "allowed_mentions": {"parse": []}}`.
- The post runs in its own goroutine with a 10-second timeout, after the request is
  stored; errors are logged without the URL (the URL contains the webhook secret)
  and never reach the agent.
- Only creation is notified. Outcomes are in the UI, the audit log and the poll.

Infra (separate change in `infra-plateful`): pass `PLATFORM_APPROVALS_WEBHOOK_URL`
to the controlplane's secrets env from the existing tfvar `alert_webhook_url`, the
Discord webhook Grafana alerts already use.

## 12. Audit

- Request events, target kind `request`, target the request id, team and
  environment of the request, details `kind`, `summary`, `reason`, `result`, `error`:
  `request.created` (the owner via the token), `request.approved`,
  `request.rejected`, `request.cancelled`, `request.superseded`,
  `request.expired` (actor `platform`), `request.executed`, `request.failed`.
- The domain event of an approved change (`config.revised`, `deploy.requested`,
  `queue.*`) is recorded as the owner via the token, with `request_id` and
  `approved_by` in its details.
- **Bypass audit moves out of `Policy.Can`.** The policy no longer publishes
  anything. `token.bypass_used` is published by the usecase right after the change is
  written, through `authz.PublishBypass(ctx, publisher, gate, actor, action,
  resource)`. A bypass token whose change fails validation leaves no bypass event,
  and a policy check that only renders a page cannot emit one. `NewPolicy` loses its
  publisher argument.

## 13. Storage and code layout

| Where | What |
|---|---|
| `migrations/000006_change_requests` | section 5 |
| `pkg/approval` | kinds, `Change`, `Pending`, `Preview`, `Line` |
| `pkg/actor` | `Approval`, `Actor.Approved()` |
| `pkg/apperror` | `err_stale` (409) |
| `app/authz` | `Gate`, `Direct`, `PublishBypass`, `request.approve` |
| `app/approvals` | entities, MySQL repository, Discord notifier, executors that decode a payload and call the owning domain, `submit` usecase (used by the domains) and `requests` usecase (read, approve, reject, cancel, sweep), JSON handlers |
| `app/envconfig`, `app/deploy`, `app/queue` | `Gate`, proposals, `ApplyApproved`, `Preview` |
| `app/identity/usecases/tokens` | `ActorOf(tokenID)` |
| `app/web/handlers/requests` and two templates | UI |

Two approvals usecases keep the dependency graph acyclic: domains depend on
`submit`; `requests` depends on the domains through executors.

## 14. Testing

- Unit, `authz`: the gate table (needs approval, bypassed, approved, open for dev and
  sessions); an approval does not replace the `protected` scope; `request.approve`
  only for maintainers and admins in a session; `PublishBypass` only for `bypassed`.
- Unit, envconfig: a gated revise or secret submits instead of writing, after full
  validation; the secret value never reaches the change; re-apply keeps other moved
  keys; a moved key fails with `err_stale`; preview diffs against now and hides
  secret values; bypass is published only with the write.
- Unit, deploy: gated deploy submits with the live id; promote runs the pinned
  release after dev moved on; a newer live deployment fails the run; rollback;
  bypass with the deployment; preview.
- Unit, queue: gated create, update (base version kept), delete (non-empty still
  refused); pause and dead letters still `err_approval_required`.
- Unit, approvals: entity transitions; repository round trip, conditional
  transition, supersede lookup, overdue; submit (202 body, supersede, clipped
  summary, notification text); approve runs as the token with the approval, records
  failures with their code, fails on a revoked token; only approvers; expired and
  twice refused; reject, cancel, visibility, counter, sweep; Discord posts without
  mentions, does not block, survives failures, logs without the URL.
- Integration: a prod deploy waits, is approved in the browser, runs, and the poll
  says `executed`; members and tokens cannot approve, admins can; a secret request
  never shows its value; re-apply on moved config and a real conflict; supersede;
  reject and cancel reach the agent; the nav counter; a queue create waits too; the
  existing bypass test now expects `202` without bypass.
- Integration test databases are closed after each test so the growing suite stays
  under MySQL's connection limit.

## 15. Out of scope

Approver ≠ owner, multi-approver rules, approval from Discord buttons, notifications
of outcomes, `?wait=` long polling (sub-project 3), approvals over MCP (sub-project 4),
batch approval, editing a pending request.

## 16. Decisions to revisit

| Decision | Cost if wrong |
|---|---|
| Supersede by subject (`config`, `secret.NAME`, `deploy`, `queue.NAME`) instead of by kind | Two config revisions on different keys cancel each other; agents must send one combined change. Changing the key is one function per domain. |
| Pause/resume and dead letters stay `err_approval_required` | An agent cannot pause a prod queue during an incident without bypass. Adding them is three more kinds in the queue domain. |
| The approve click runs the change synchronously | A slow domain call holds the approver's browser; a crash mid-run leaves `approved` until the sweeper fails it after 10 minutes. |
| Polling needs `read` | A `deploy`+`protected` token without `read` cannot learn its outcome; adding `/api/v1/requests/:id` to the read-free list is one line. |
| `token.bypass_used` is now emitted after the write, not at the check | Audit queries that expected an event for failed bypass attempts see fewer events. |

## 17. Done when

An agent's token without bypass asks to deploy `hello` to prod and gets `202` with an
`approve_url`; Discord shows the message; the owner opens the link, sees the diff
against what runs now, approves, and the agent's next poll says `executed` with the
deployment; the audit log shows the request, the approval and
`deploy.requested` by "owner via token" with `approved_by`.

# Release changelog

Status: draft, 2026-10-19. Mockups: https://claude.ai/artifact/G3AtRmwwTunJZyrnVptwYC

## Problem

A release says which commit it was built from (`releases.commit_sha`) and nothing about what changed. When someone promotes to prod, the change request shows `release #5 → #8` and the reviewer has to open GitHub to learn what those three releases carry. An agent asked "what goes out if I promote?" has no answer either.

## Decisions (user, 2026-10-19)

1. The changelog is **frozen per release**. When a release is built, the platform records the commits between the previous release of the same service and the new one, and keeps them with the release. It is never recomputed.
2. Promotion shows what ships: every release between the one live in the target environment and the candidate, with their frozen changelogs.
3. The auto-build on push keeps dispatching by branch name. What matters is that the history from the last release to the new one is complete. Because the SHA is only known when the build finishes and builds can finish out of order or come from other branches, release numbers do not always follow commit order; the rules below are built around that.
4. Repositories may live in personal accounts or orgs. Nothing here depends on which.
5. Writing back to GitHub (Deployments API, PR comments) is a later step, not part of this design. This design needs no new GitHub App permission: Contents read and Pull requests read are already granted.

## What the user sees

- **Release page (new):** `/services/:service/releases/:number`. Header with status, ref@sha, trigger, age. A "Changelog since rN" card grouping commits by pull request, each PR tagged by its conventional-commit type, with a "compare on GitHub" link. Side cards: build (commit, image@digest, workflow run, services it was shared to) and where it runs (per environment, live release and how many releases behind). A "Promote to prod" button when the release is live on dev and not on prod. (Mockup board B.)
- **Releases list:** gains a "Changes" column, `5 commits since r7`, linking to the release page. The row number links there too. No inline expansion. *(My call: the user approved the mockups without picking A or B. B carries more, and the list keeps a pointer to it.)*
- **Change request page:** for promote, deploy and rollback, a "What ships to prod" (or "to dev") card below "What changes": a summary line (`3 releases · 14 commits · 6 pull requests between r5 and r8`), then one block per release, newest first, each listing its PRs and the commits that came with no PR. A rollback reads "What leaves prod" and lists the releases being rolled back over. (Mockup board "Change request".)
- **API and MCP:** `GET /api/v1/releases/:id` (`getRelease`) gains a `changelog` object. `GET /api/v1/requests/:id` (`getRequest`) gains `preview`, the same data the request page renders, including `ships`. The MCP tools pick both up from the OpenAPI metadata, no new tool. (Mockup board "MCP / API shape".)

## Model

A new table, `release_changelogs`, one row per release that has one:

| column | meaning |
|---|---|
| `release_id` PK | the release |
| `base_release_id` | the previous release it was compared with, empty for the first |
| `base_sha`, `head_sha` | the two commits compared |
| `state` | `pending`, `ahead`, `first`, `diverged`, `unavailable`, `failed` |
| `ahead_by`, `behind_by` | from the compare API |
| `truncated` | more commits than were stored |
| `compare_url` | the GitHub compare page |
| `commits` | JSON: `[{sha, title, type, author, committed_at, pull_request: {number, title, url} or null}]`, newest first |
| `attempts`, `last_error`, `updated_at` | for retries |

`title` is the first line of the commit message. `type` is the conventional-commit prefix (`feat`, `fix`, `chore`, …) or `other`. `author` is the GitHub login when GitHub resolves one, else the git author name.

A separate table rather than columns on `releases`: the changelog is filled after the release is built, may be retried, and is large compared with the release row.

## How the changelog is built

1. `ReleasesUsecase.Built` already publishes `release.built`, and `share` publishes `release.shared` for each sibling. A new `changelog` subscriber handles both. Events are delivered synchronously inside the build's callback request (`internal/eventbus/eventbus.go`), so the subscriber only writes the `pending` row and returns; the GitHub calls run in a background goroutine with their own timeout, never in the callback.
2. It picks the **base**: the release of the same service with `status = built` and a non-empty `commit_sha` that **finished most recently before this one finished** (`finished_at`), whatever its number. "Since the previous release" means what was new since the last release that existed when this one was built. *(My call: by finish time rather than by number, because numbers are handed out at request time and builds can finish out of order.)* Shared releases count: they belong to the sibling service.
3. It writes the row as `pending`, then fetches:
   - `GET /repos/{repo}/compare/{base_sha}...{head_sha}` with the installation token (existing `githubapp.Client`, new `Compare` method).
   - For each commit, the PR that merged it: `GET /repos/{repo}/commits/{sha}/pulls`, at most 30 calls per changelog, the rest left with `pull_request: null`.
4. State rules:
   - no base → `first`, commits = the head commit only;
   - release without `commit_sha` (registered image) → `unavailable`, no row needed: the API reports it from the release itself;
   - compare `status` `ahead` → `ahead`; `identical` → `ahead` with no commits; `diverged` or `behind` → `diverged`, commits as returned, `ahead_by`/`behind_by` stored;
   - more than 250 commits (the compare API's limit) → `truncated = true`;
   - GitHub error → stays `pending` with `last_error` and `attempts` incremented. New: the release sweeper (`ReleasesUsecase.Sweep`, ticking every minute) also picks `pending` changelogs whose `updated_at` is older than `2^attempts` minutes and retries them; after 5 attempts the row becomes `failed`. Today's sweeper has no retry logic: this is new code. The same sweep also covers a controlplane restart that killed a background fetch.
5. Same `(repo, base_sha, head_sha)` already computed for a sibling's release → copy the commits, no GitHub call.

The subscriber never fails the build or the deploy: a missing changelog is shown as "changelog unavailable" with the reason.

## How promotion assembles "what ships"

Stitching the per-release changelogs together is wrong when releases are not one straight line: a release built from a feature branch, or builds that finished out of order, would show commits that do not ship or count them twice. So "what ships" is its own comparison, grouped by release for display. *(My call: it keeps the view the user approved — r6, r7, r8 together — while being correct.)*

1. **When:** computed once, when the change request is created, and **stored with the request** (new column `change_requests.ships` JSON). It is frozen like the release changelog, so an approved or executed request still shows what it shipped. For direct deploys that need no approval (dev), it is computed for the deployment's preview the same way but not stored.
2. **Compare:** live release SHA `L` in the target environment against candidate SHA `C`, one `compare` call (plus PR lookups, same caps and reuse of rows already fetched for the same SHA pair). No live release → `state: first`, the candidate's own changelog.
3. **Direction:** compare `ahead` → `forward` ("what ships"); `behind` (a rollback, or promoting an older release) → `backward` ("what leaves"), listing the commits being removed; `diverged` → both lists, with a warning.
4. **Grouping:** each commit is attributed to the lowest-numbered release between the two (by number, `L` exclusive … `C` inclusive) whose frozen changelog contains its SHA; commits found in none go to a final "not in any release changelog" group. The summary line counts distinct SHAs.
5. **Failure:** if GitHub is unreachable when the request is created, the request is created anyway with `ships.state = failed` and the reason; the request page offers no retry in v1.
6. Config-only requests and removals carry no `ships`.

`approval.Preview` stays domain-neutral: it gains `Ships *Ships` with plain fields, filled only by the deploy executor. `getRequest` returns the stored `ships` for every status, not only pending, and the web page shows it for every status too.

## Errors and edge cases

- Force-push or a build of a non-default branch → `diverged`, rendered with a warning: "r8 is not a descendant of r7; showing the commits GitHub reports".
- The App lost access to the repository → `failed`, `last_error` names it.
- A changelog still `pending` when a request is opened → the request page shows "still loading" for that release; the API returns the state as is.
- Monorepos: commits are not filtered by path. Out of scope until services carry a path.

## Non-goals

- Writing anything to GitHub.
- An LLM summary on the server: agents summarise from the API.
- Backfilling changelogs for releases built before this ships. *(My call: existing releases show "no changelog"; a one-off backfill can be added if the user wants it.)*
- Changelogs across services that do not share an image.

## Testing

- Unit: base selection by finish time (including out-of-order finishes), state rules, conventional-commit parsing, retry backoff, ships grouping for forward / backward / diverged / no live release and for a feature-branch release in between.
- Repository: `release_changelogs` round trip, JSON column.
- `githubapp.Compare` and PR lookup against an `httptest` server with recorded GitHub responses.
- Integration: built callback → changelog row; request preview over the API returns `ships`.
- Route-parity and OpenAPI tests pick up the new fields.

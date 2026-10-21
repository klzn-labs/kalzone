# Services list and service overview: releases, deploys and prod health

Date: 2026-10-21. Status: approved; the user asked to build without further questions, so the calls under "Builder's calls" are mine.

Mockups the user chose from:
- Services list, option C "Rows by team" with collapsible teams: https://claude.ai/artifact/3bJN5jLWGHrSd3wdNNzTby
- Service overview, prod only: https://claude.ai/artifact/5ECDM2eTNNrJHhTFDJxxm5 (toggle "All OK" / "DB ran hot")

## Why

When the user opens Kalzone they want to know two things about their services:
are releases and deploys going through, and are the service's metrics ok. Today
the services list is a grid of grey cards with a description and two release
chips, and the service page opens on an About box, environment cards and a
danger zone. Neither answers either question, and neither shows any metric.

The metrics the user looks at to decide "ok" are response time, CPU and DB CPU.
When they are fine the user does not care about them; they only want them to
take space when something is wrong.

## Goals

1. **Services list** answers, per service and at a glance: is it building, is
   it deploying, what runs in prod, and is anything broken.
2. **Service overview** (the first tab of a service) answers the same for one
   service, for **prod only**: what is live, whether its builds and prod
   deploys went through, and whether its metrics are within their limits.
3. **Metrics are quiet when fine and loud when not.** Within limits they are one
   line; past a limit, that metric opens up with its peak and a chart.
4. **One blinking red mark** on a service in the list is the signal that it has
   a problem. Nothing else on the list uses red.
5. **Limits have a platform default and can be set per service.**

## Non-goals

- Changing the Releases, Dev and Prod tabs. They keep their current content.
  The service tabs stay **Overview | Releases | Dev | Prod**.
- Alerting outside the console (Discord, email). The existing rollback alerts
  are untouched.
- Metrics for dev. Dev never appears on the Overview tab.
- Per-database CPU. MySQL is one shared server; see Constraints.

## Anti-goals (would make this a failure even if it works)

- Dev information on the service Overview tab. The user was explicit: the
  overview is prod.
- Metrics that take space or draw the eye while they are within limits.
- Red or blinking used for anything that is not broken (a pending approval, a
  deploy in progress or an older release in prod are not errors).
- A page the user cannot read the status of without hovering or clicking.

## The services list

Layout from the chosen mockup (option C):

- Title row: "Services", a subline with the counts ("6 services · 2 need
  attention · 1 deploying"), and the New service button.
- One section per team, in the existing team order. The header is the team
  name, its service count and either "N need attention" (in the red ink) or
  "all fine" (grey). Each section **collapses** with a chevron on the header.
  A collapsed team still shows the header, so its attention count stays visible.
- Inside a section, one row per service, broken services first:
  - status mark: a **blinking red dot** when broken (see "Broken" below),
    otherwise a small faint green dot;
  - service initials tile, name, and below it the prod host or the slug;
  - **Latest release**: its number and when it was built;
  - **Build**: a pill, Built (green tint), Building (spinner), Failed (red tint);
  - **In prod**: the live release number and since when;
  - **Now**: what is happening, in this order and all that apply: the reason it
    is broken in red text ("DB CPU over 70%", "Build #9 failed", "Deploy #17 to
    prod failed"), a spinner with "Deploying #10 to prod" while a prod
    deployment is pending or in progress, and an amber pill "Promote #26 waits
    for you" when a prod change request waits on the viewer. A grey dash when
    none apply.
- The existing filter tabs (All, My teams, per team, Platform) are dropped in
  favour of the team sections. The Kalzone control plane row (today's platform
  card) appears as its own section "Platform" at the end, read-only, with its
  live commit in the In prod column. *(Builder's call how far the platform row
  follows the other rows; it has no releases or metrics of its own here.)*

The blinking is CSS only (opacity pulse, about 1 s) and stops under
`prefers-reduced-motion`, where the dot stays solid red.

## The service Overview tab

Replaces today's Overview tab content. Prod only. Top to bottom:

1. **Header**: service name, a Live pill with the prod release and since when
   ("#25 since 20 Oct 06:53"), the prod host. Actions on the right: Roll back
   (secondary) and, when a prod change request for this service waits on the
   viewer, a primary "Review promotion of #N" linking to that request.
   Otherwise the primary action is Deploy to prod (linking to the Prod tab's
   deploy form).
2. **Health strip**, one of:
   - **All within limits**: a single green-tinted line, "Prod is healthy",
     followed by the three current values ("Response p95 181 ms · CPU 12% ·
     DB CPU 38%") and "within limits for the last 24 hours" on the right.
   - **Something went over**: an amber (went over in the last 24 h, fine now)
     or red (over the limit now) block for each metric that crossed its limit,
     with one sentence (peak, when, how long over, current value) and a 24-hour
     chart of that metric with its limit as a dashed line. The metrics that
     stayed within limits remain as one line under it ("Response p95 181 ms ok
     · CPU 12% ok").
   - **No data**: when Prometheus has no samples for a metric (no traffic, no
     database attached), that metric reads "no data" in grey and never counts
     as broken. A service with no database shows no DB CPU at all.
3. **Releases** (left, wider): the latest 4 releases with number, when, the
   change (commit title or ref), Build pill, and **In prod** pill: Live, Was
   live, Waits for you, Never. "All N" links to the Releases tab.
4. **Prod deploys** (right): the latest 4 prod deployments with result icon,
   release, deployment number, trigger, strategy and duration, and when. The
   header says "N in 14 days · M failed".

Today's Overview content moves as follows *(my call, see Open questions)*:
the About details and the Edit form, the Resources table and the Danger zone go
to a section at the bottom of the Overview tab titled "Settings", collapsed by
default. The dev and prod environment cards are dropped; the Dev and Prod tabs
cover them.

## Broken, and the limits

A service is **broken** when any of these is true:

- **Metric over its limit now**: the latest value (5-minute window) of
  response p95, CPU or DB CPU in **prod** is at or above its limit.
- **Latest build failed**: the newest release of the service failed to build,
  and no newer release built since.
- **Latest prod deploy failed**: the newest finished prod deployment of the
  service failed. A later successful prod deployment of any trigger, a
  rollback included, clears it; cancelled deployments are ignored; change
  requests never count.

A metric that went over in the last 24 hours but is fine now is **not broken**:
it shows amber on the Overview tab and nothing on the list *(my call: the
user's "blinking red = has errors" reads as "now")*.

**Limits** (all prod):

| Metric | Query basis (already used by the service dashboards) | Default limit |
| --- | --- | --- |
| Response time, p95 | `histogram_quantile(0.95, …traefik_service_request_duration_seconds_bucket{service=~"<slug>-prod-(blue\|green)@file"}…)` | 300 ms |
| CPU | the busiest single prod container, `max(rate(container_cpu_usage_seconds_total{name=~"<slug>-prod-(blue\|green)"}))`, as a share of the CPU the prod config grants one container (`spec.cpu_millis`), so a blue-green overlap or several replicas never double it | 70% |
| DB CPU | CPU of the shared MySQL container as a percentage of one core (the unit the existing MySQL panel shows), only for services with a prod database attached | 70% |

- The defaults are platform config, overridable by environment variables in the
  existing `PLATFORM_*` style *(builder picks the names)*.
- Each service can override any of the three limits. An unset override means
  "use the default". Overrides are stored on the service, changed through the
  existing service update (API `PATCH /api/v1/services/:slug` and the MCP tool
  that wraps it) and the Edit form under Settings, by the same people who can
  edit the service today. Every change is audited like other service edits.

## Constraints

- **DB CPU is shared.** MySQL is one container serving every schema (Plateful
  and LegendsGrid today). There is no per-schema CPU. The label says so: "DB
  CPU · shared MySQL". A hot database therefore marks every attached service as
  broken; that is accepted for now.
- **Metrics come from Prometheus through Grafana.** The control plane already
  runs instant queries through the Grafana datasource proxy
  (`app/deploy/repositories/metrics`, `Querier.Instant`). The 24-hour charts
  need range queries (`query_range`), which the client and
  `internal/fakegrafana` do not support yet.
- **Prometheus can be down or slow.** The list and the overview must still
  render with release and deploy data; metrics then read "no data" and never
  mark a service broken. Queries for the list run for every visible service, so
  they need a short timeout and should be done concurrently or batched (one
  query per metric grouped by service rather than one per service is
  preferred).
- **Project rules**: functions of at most 20 lines, at most two levels of
  nesting, no comments in any file; one commit per layer (storage, rules,
  handlers with wiring and integration tests); the gitignored
  `internal/zzdemo` harness is used for screenshots.
- Look and feel: the framed-sheet style already live (IBM Plex Sans, teal only
  on data, near-black primary button, status pills as pale tints).

## Decided by the user

- Tabs stay Overview | Releases | Dev | Prod.
- The Overview tab is prod only.
- The list is grouped by team, each team collapsible.
- One blinking red mark per broken service is enough on the list.
- The mark fires for a metric over its limit, a failed build or a failed
  deploy.
- Limits: a platform default, configurable per service.

## Left to the builder

- Exact copy, spacing, and how the platform row looks on the list.
- Whether a team's collapsed state is remembered (browser storage is fine).
- The query batching strategy and timeouts.
- Chart drawing (server-rendered SVG like the overview chart is preferred).

## Builder's calls (after the review)

- About, Edit form, Resources and Danger zone go to a collapsed "Settings"
  section at the bottom of the Overview tab.
- Health is console-only for now. The rules live in an observe usecase, not in
  the web handlers, so an API field can expose them later. The service API
  returns the per-service limit overrides.
- DB CPU is a percentage of one core; no infra change.
- CPU uses the busiest prod container against one container's `cpu_millis`.
- The list reads only the current value of each metric; the 24-hour history is
  queried only on the Overview tab.
- A limit override of 0 means "use the platform default"; overrides are
  validated as 1-60000 ms and 1-400% (DB CPU can exceed one core).

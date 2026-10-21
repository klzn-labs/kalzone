# Rollback alerts, Kalzone naming and the controlplane catalog entry

Date: 2026-10-21. Mockups: https://claude.ai/artifact/26hwcZuu4ZMwtZALgpGRP7 (option A chosen).

## Problem

A controlplane rollback on 2026-10-21 fired "Deploy rolled back" in Discord with Grafana's default
body: `A=29, C=1`, labels, and a folder named Plateful. It did not say why, which versions, or where
to look. The controlplane is also invisible in Kalzone because kalzone-infra deploys it directly.

## Decisions

- Graph: a dashboard link opened on the ten minutes around the rollback. No rendered images.
- Rename the monitoring names from Plateful to Kalzone now: metric and pushgateway job names,
  Grafana folders, rule group, contact point, dashboard uids and titles. Plateful's own sign-up
  alert stays under Plateful because it is that app's metric. `/opt/plateful`, the compose
  project, the `plateful` network, container labels and volumes move with the planned infra move.
- controlplane shows in the catalog as a read-only card (option A) with a Platform filter tab and a
  detail page: live version, health, recent deploys with rollback reasons, links.

## Design

### deployctl (kalzone-infra)

- Metrics: `kalzone_deploy_info`, `kalzone_deploy_last_rollback_timestamp_seconds`,
  `kalzone_traffic_weight_percent`; jobs `kalzone_deploy`, `kalzone_rollback`, `kalzone_traffic`.
- The rollback series carries labels `version`, `previous`, `step` (start, ready, smoke, gate,
  other), `reason` (one line, at most 200 characters), `run_url`, `strategy` and `at_ms`.
- The gate's latency failure states the baseline: `p95 latency 331ms above limit 293ms (baseline
  93ms), 30 requests, 0.00% 5xx`.
- History: every outcome is appended to `state/<service>.history.json`, newest first, at most 20
  entries of `{at, version, outcome, step, reason, strategy, colour, run_url}`. `remove` deletes it.

### Grafana (kalzone-infra)

- Rule group `kalzone` in folder Kalzone; the sign-up rule moves to group `plateful` in folder
  Plateful. The rollback rule gets label `kind: rollback` and annotations built from the labels.
- Contact point `kalzone-alerts` replaces `plateful-webhook` (deleted through
  `deleteContactPoints`); a `kalzone-rollbacks` contact point with a custom Discord title and
  message, no resolve message, routed on `kind=rollback`.
- Dashboards: provider and folder Kalzone, uids `kalzone-*`, titles `Kalzone / …`. The message
  links `/grafana/d/kalzone-http?var-service=<service>&time=<at_ms>&time.window=600000`.

### Kalzone

- The agent reads `state/controlplane.json` and `state/controlplane.history.json` on its status tick,
  observes the live container and reports it with `PUT /agent/v1/platform/controlplane`.
- The control plane keeps the latest report in `platform_components` (name, JSON payload, agent,
  observed_at) and renders it read-only: a card first in the catalog grid, a Platform tab, and
  `/platform/controlplane`. Nothing on it is editable.
- Per-service dashboards read `kalzone_traffic_weight_percent`.

## Rollout

1. kalzone-infra PR: deployctl, Grafana provisioning, dashboards, e2e. Apply the stack; delete the
   old pushgateway groups (`plateful_*` jobs) and the empty Plateful dashboard folder.
2. kalzone PR: storage, usecase, agent + handlers + web. Release the controlplane (its deploy runs
   the new deployctl and writes the first history entry), roll the agent image through tfvars.

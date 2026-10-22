package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

const (
	devAfter      = 15 * time.Minute
	prodAfter     = 6 * time.Hour
	rollbackAfter = 15 * time.Minute
	pendingFor    = 30 * day
)

type run struct {
	env     string
	rel     release.Release
	at      time.Time
	status  deploy.Status
	trigger deploy.Trigger
	by      person
	failure *deploy.Failure
}

func (u SeedUsecase) history(ctx context.Context, s catalog.Service, w workload, built []release.Release, now time.Time) error {
	for _, r := range runsOf(w, built) {
		if err := u.deploy(ctx, s, r); err != nil {
			return err
		}
	}
	if w.prod == w.releases || w.rolledBack > 0 {
		return nil
	}
	return u.request(ctx, s, built[len(built)-1], now)
}

func runsOf(w workload, built []release.Release) []run {
	runs := []run{}
	for n := max(1, w.releases-3); n <= w.releases; n++ {
		runs = append(runs, run{env: "dev", rel: built[n-1], at: built[n-1].FinishedAt.Add(devAfter), status: deploy.StatusSucceeded, trigger: deploy.TriggerPush, by: people[2]})
	}
	for n := max(1, w.prod-4); n <= w.prod; n++ {
		runs = append(runs, run{env: "prod", rel: built[n-1], at: built[n-1].FinishedAt.Add(prodAfter), status: deploy.StatusSucceeded, trigger: deploy.TriggerManual, by: people[0]})
	}
	runs = append(runs, rollbackOf(w, built)...)
	slices.SortStableFunc(runs, func(a, b run) int { return a.at.Compare(b.at) })
	return runs
}

func rollbackOf(w workload, built []release.Release) []run {
	if w.rolledBack == 0 {
		return nil
	}
	failedAt := built[w.rolledBack-1].FinishedAt.Add(prodAfter)
	failure := &deploy.Failure{Class: "unhealthy", Container: w.slug + "-prod-green", HealthLog: []string{"GET /healthz 503 after 30s"}, Logs: []string{"error: template cache missing for locale pt-PT"}}
	return []run{
		{env: "prod", rel: built[w.rolledBack-1], at: failedAt, status: deploy.StatusFailed, trigger: deploy.TriggerManual, by: people[1], failure: failure},
		{env: "prod", rel: built[w.prod-1], at: failedAt.Add(rollbackAfter), status: deploy.StatusSucceeded, trigger: deploy.TriggerRollback, by: people[1]},
	}
}

func (u SeedUsecase) deploy(ctx context.Context, s catalog.Service, r run) error {
	started := r.at.Add(-2 * time.Minute)
	d, err := u.store.Deployments.Create(ctx, deploy.Deployment{
		ID: uuid.NewUUID(), ServiceID: s.ID, Environment: r.env, ReleaseID: r.rel.ID, ReleaseNumber: r.rel.Number,
		ImageRef: r.rel.Image + "@" + r.rel.Digest, ConfigRevision: 1, Strategy: deploy.StrategyBlueGreen, Trigger: r.trigger,
		Status: r.status, RequestedBy: r.by.id, Agent: "demo", CreatedAt: started.Add(-time.Minute), StartedAt: &started, FinishedAt: &r.at, Failure: r.failure,
	})
	if err != nil {
		return err
	}
	details := map[string]any{"deployment": d.Number, "release": r.rel.Number, "config": 1}
	u.event(ctx, s, r.by, "deploy.requested", r.env, d.CreatedAt, details)
	u.event(ctx, s, r.by, "deploy."+string(r.status), r.env, r.at, details)
	return nil
}

func (u SeedUsecase) request(ctx context.Context, s catalog.Service, rel release.Release, now time.Time) error {
	by := people[2]
	payload, err := json.Marshal(map[string]any{"kind": approval.KindDeploy, "service": s.Slug, "environment": "prod", "release_id": rel.ID, "strategy": "bluegreen"})
	if err != nil {
		return err
	}
	r := approvals.Request{
		ID: uuid.NewUUID(), Kind: approval.KindDeploy, Subject: "deploy", ServiceID: s.ID, ServiceSlug: s.Slug, TeamID: s.OwnerTeamID,
		Environment: "prod", Summary: fmt.Sprintf("deploy release #%d to prod", rel.Number), Payload: payload, RequestedBy: by.id,
		RequesterLogin: by.login, Status: approvals.StatusPending, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(pendingFor),
	}
	if err := u.store.Requests.Create(ctx, r); err != nil {
		return err
	}
	e := auditOf(s, by, "request.created", "prod", r.CreatedAt, map[string]any{"summary": r.Summary})
	e.TargetKind, e.TargetID, e.TargetName = events.TargetRequest, r.ID, s.Slug
	return u.store.Events.Insert(ctx, e)
}

func auditOf(s catalog.Service, p person, action, env string, at time.Time, details map[string]any) audit.Event {
	kind, id, name := targetOf(s)
	return audit.Event{
		ID: uuid.NewUUID(), At: at, ActorID: p.id, ActorLogin: p.login, Action: action, TargetKind: kind,
		TargetID: id, TargetName: name, TeamID: s.OwnerTeamID, Environment: env, Details: details,
	}
}

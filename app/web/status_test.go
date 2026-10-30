package web_test

import (
	"context"
	"testing"
	"time"

	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	database "github.com/klzn-labs/kalzone/app/database/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	observe "github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/health"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

type sources struct {
	releases []release.Release
	deploys  []deploy.Deployment
	waiting  []approvals.Request
	targets  []health.Target
	over     bool
}

func (s *sources) List(context.Context, string, int) ([]release.Release, error) {
	return s.releases, nil
}

func (s *sources) History(context.Context, string, string, int) ([]deploy.Deployment, error) {
	return s.deploys, nil
}

func (s *sources) Current(context.Context, string, string) (envconfig.Revision, error) {
	return envconfig.Revision{Spec: envconfig.Spec{CPUMillis: 500}}, nil
}

func (s *sources) AttachedTo(context.Context, string) ([]database.Database, error) {
	return []database.Database{{Name: "lolgrid", Environment: "prod"}}, nil
}

func (s *sources) Awaiting(context.Context, actor.Actor) ([]approvals.Request, error) {
	return s.waiting, nil
}

func (s *sources) Day(_ context.Context, t health.Target) observe.Health {
	s.targets = append(s.targets, t)
	return s.health()
}

func (s *sources) NowAll(_ context.Context, targets []health.Target) map[string]observe.Health {
	s.targets = append(s.targets, targets...)
	return map[string]observe.Health{targets[0].Slug: s.health()}
}

func (s *sources) health() observe.Health {
	value := 100.0
	if s.over {
		value = 500
	}
	return observe.Health{Readings: []observe.Reading{{Metric: observe.MetricResponse, Limit: 300, Now: value, Held: value, Known: true}}}
}

func statuses(s *sources) web.Statuses {
	return web.NewStatuses(web.StatusSources{Releases: s, Deployments: s, Configs: s, Databases: s, Health: s, Requests: s})
}

func deployed(number, rel int, status deploy.Status) deploy.Deployment {
	at := clock.Now().Add(-time.Hour)
	return deploy.Deployment{Number: number, ReleaseNumber: rel, Status: status, FinishedAt: &at}
}

var grid = catalog.Service{ID: "s1", Slug: "legendsgrid", Limits: catalog.Limits{ResponseMS: 500}}

func Test_AFailedBuildAndAFailedProdDeployAreProblems(t *testing.T) {
	s := &sources{
		releases: []release.Release{{Number: 9, Status: release.StatusFailed}, {Number: 8, Status: release.StatusBuilt}},
		deploys:  []deploy.Deployment{deployed(17, 8, deploy.StatusFailed), deployed(16, 7, deploy.StatusSucceeded)},
	}

	st, err := statuses(s).Day(context.Background(), actor.Actor{}, grid, "prod")

	require.NoError(t, err)
	require.Equal(t, []string{"Build #9 failed", "Deploy #17 to prod failed"}, st.Problems)
	require.Equal(t, 7, st.Live().ReleaseNumber)
	require.Equal(t, health.Target{Slug: "legendsgrid", CPUMillis: 500, Database: true, Limits: catalog.Limits{ResponseMS: 500}}, s.targets[0])
}

func Test_ASuccessfulRollbackClearsAFailedDeployAndCancelledOnesAreIgnored(t *testing.T) {
	s := &sources{deploys: []deploy.Deployment{
		deployed(19, 9, deploy.StatusCancelled), deployed(18, 7, deploy.StatusSucceeded), deployed(17, 8, deploy.StatusFailed),
	}}

	all, err := statuses(s).Now(context.Background(), actor.Actor{}, []catalog.Service{grid}, "prod")

	require.NoError(t, err)
	require.Empty(t, all[0].Problems)
	require.False(t, all[0].Broken())
	require.Equal(t, web.DeployStats{Total: 2, Failed: 1}, all[0].Stats())
	require.Len(t, all[0].FinishedDeploys(4), 2)
}

func Test_AMetricOverItsLimitMakesTheServiceBroken(t *testing.T) {
	s := &sources{over: true}

	all, _ := statuses(s).Now(context.Background(), actor.Actor{}, []catalog.Service{grid}, "prod")

	require.True(t, all[0].Broken())
	require.Equal(t, []string{"Response p95 over 300 ms for 5 min"}, all[0].Problems)
}

func Test_EachReleaseKnowsWhetherItReachedProd(t *testing.T) {
	s := &sources{
		releases: []release.Release{{ID: "r27", Number: 27}, {ID: "r26", Number: 26}, {ID: "r25", Number: 25}, {ID: "r23", Number: 23}},
		deploys:  []deploy.Deployment{deployed(16, 25, deploy.StatusSucceeded), deployed(15, 23, deploy.StatusSucceeded)},
		waiting: []approvals.Request{
			{Kind: "promote", ServiceSlug: "legendsgrid", Environment: "prod", Payload: []byte(`{"kind":"promote","release_id":"r26"}`)},
			{Kind: "promote", ServiceSlug: "web", Environment: "prod", Payload: []byte(`{"release_id":"r27"}`)},
		},
	}

	st, _ := statuses(s).Day(context.Background(), actor.Actor{}, grid, "prod")

	require.NotNil(t, st.Waiting)
	require.Equal(t, []string{web.InProdNever, web.InProdWaiting, web.InProdLive, web.InProdWas},
		[]string{st.InProd(st.Releases[0]), st.InProd(st.Releases[1]), st.InProd(st.Releases[2]), st.InProd(st.Releases[3])})
}

func Test_ProdIsTheFirstProtectedEnvironment(t *testing.T) {
	prod, ok := web.ProdEnvironment([]catalog.Environment{{Slug: "dev"}, {Slug: "prod", Protected: true}})
	_, none := web.ProdEnvironment([]catalog.Environment{{Slug: "dev"}})

	require.True(t, ok)
	require.Equal(t, "prod", prod.Slug)
	require.False(t, none)
}

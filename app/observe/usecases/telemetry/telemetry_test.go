package telemetry_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/telemetry"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type catalog struct{}

func (catalog) Services(context.Context) ([]entities.Service, error) {
	return []entities.Service{
		{Slug: "api", TeamID: "t1", Databases: []entities.Database{{Schema: "shop", User: "api_prod"}}},
		{Slug: "other", TeamID: "t2"},
	}, nil
}

type directory struct{}

func (directory) TeamID(_ context.Context, slug string) (string, error) {
	ids := map[string]string{"alpha": "t1", "beta": "t2", "empty": "t3"}
	if id, ok := ids[slug]; ok {
		return id, nil
	}
	return "", apperror.NotFound("team not found")
}

func (directory) Environments(context.Context) ([]string, error) {
	return []string{"dev", "prod"}, nil
}

type upstream struct {
	path string
	form url.Values
	call entities.LokiCall
}

func (u *upstream) Prometheus(_ context.Context, path string, form url.Values) (entities.TelemetryAnswer, error) {
	u.path, u.form = path, form
	return entities.TelemetryAnswer{Status: 200}, nil
}

func (u *upstream) Loki(_ context.Context, call entities.LokiCall) (entities.TelemetryAnswer, error) {
	u.call = call
	return entities.TelemetryAnswer{Status: 200}, nil
}

func setup(key string) (*upstream, telemetry.TelemetryUsecase) {
	u := &upstream{}
	return u, telemetry.NewTelemetryUsecase(key, "all-token", catalog{}, directory{}, u)
}

func prometheus(uc telemetry.TelemetryUsecase, team, path string, form url.Values) error {
	_, err := uc.Prometheus(ctx, telemetry.PrometheusRequest{Team: team, Token: uc.Token(team), Path: path, Form: form})
	return err
}

func Test_Token_DiffersPerTeamAndPerKey(t *testing.T) {
	_, one := setup("k1")
	_, two := setup("k2")

	require.NotEqual(t, one.Token("alpha"), one.Token("beta"))
	require.NotEqual(t, one.Token("alpha"), two.Token("alpha"))
}

func Test_Prometheus_RefusesAnotherTeamsToken(t *testing.T) {
	_, uc := setup("k1")

	_, err := uc.Prometheus(ctx, telemetry.PrometheusRequest{Team: "beta", Token: uc.Token("alpha"), Path: "/api/v1/query"})

	require.Equal(t, apperror.CodeUnauthenticated, apperror.CodeOf(err))
}

func Test_Prometheus_IsOffWithoutAKey(t *testing.T) {
	_, uc := setup("")

	require.Equal(t, apperror.CodeNotConfigured, apperror.CodeOf(prometheus(uc, "alpha", "/api/v1/query", nil)))
}

func Test_Prometheus_ScopesTheQueryToTheTeamsServices(t *testing.T) {
	up, uc := setup("k1")

	require.NoError(t, prometheus(uc, "alpha", "/api/v1/query_range", url.Values{"query": {"traefik_service_requests_total"}, "step": {"60"}}))

	require.Equal(t, "/api/v1/query_range", up.path)
	require.Equal(t, `traefik_service_requests_total{service=~"(?:api)-(?:dev|prod)-(?:blue|green)@file"}`, up.form.Get("query"))
	require.Equal(t, "60", up.form.Get("step"))
}

func Test_Prometheus_ScopesMatchersAndAddsThemWhenMissing(t *testing.T) {
	up, uc := setup("k1")

	require.NoError(t, prometheus(uc, "alpha", "/api/v1/label/__name__/values", nil))
	require.Contains(t, up.form["match[]"], `{__name__=~"mysql_info_schema_processlist_.*",mysql_user=~"(?:api_prod)"}`)

	require.NoError(t, prometheus(uc, "alpha", "/api/v1/series", url.Values{"match[]": {"kalzone_database_attachment"}}))
	require.Equal(t, []string{`{__name__="kalzone_database_attachment",service=~"(?:api)"}`}, up.form["match[]"])
}

func Test_Prometheus_RefusesWritesAdminAndTheAllTeamsToken(t *testing.T) {
	_, uc := setup("k1")

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(prometheus(uc, "alpha", "/api/v1/admin/tsdb/delete_series", nil)))
	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(prometheus(uc, "alpha", "/api/v1/write", nil)))
	_, err := uc.Prometheus(ctx, telemetry.PrometheusRequest{Team: entities.AllTeams, Token: "all-token", Path: "/api/v1/query"})
	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
}

func Test_Prometheus_RejectsAQueryThatIsNotPromQL(t *testing.T) {
	_, uc := setup("k1")

	require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(prometheus(uc, "alpha", "/api/v1/query", url.Values{"query": {"sum("}})))
}

func loki(uc telemetry.TelemetryUsecase, team, path string) error {
	return lokiWith(uc, team, uc.Token(team), path)
}

func lokiWith(uc telemetry.TelemetryUsecase, team, token, path string) error {
	_, err := uc.Loki(ctx, telemetry.LokiRequest{Team: team, Token: token, Call: entities.LokiCall{Method: "GET", Path: path}})
	return err
}

func Test_Loki_ReadsOnlyTheTeamsTenants(t *testing.T) {
	up, uc := setup("k1")

	require.NoError(t, loki(uc, "alpha", "/loki/api/v1/query_range"))

	require.Equal(t, []string{"api-dev", "api-prod"}, up.call.Tenants)
}

func Test_Loki_GivesATeamWithoutServicesATenantThatHoldsNothing(t *testing.T) {
	up, uc := setup("k1")

	require.NoError(t, loki(uc, "empty", "/loki/api/v1/labels"))

	require.Equal(t, []string{entities.NoTenant}, up.call.Tenants)
}

func Test_Loki_GivesTheAllTokenEveryTenant(t *testing.T) {
	up, uc := setup("k1")

	require.NoError(t, lokiWith(uc, entities.AllTeams, "all-token", "/loki/api/v1/label/service/values"))

	require.Equal(t, []string{"api-dev", "api-prod", "other-dev", "other-prod", "controlplane", "platform", "fake"}, up.call.Tenants)
}

func Test_Loki_RefusesPushDeleteAndPathTricks(t *testing.T) {
	_, uc := setup("k1")

	for _, path := range []string{"/loki/api/v1/push", "/loki/api/v1/delete", "/loki/api/v1/label/../push", "/config"} {
		require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(loki(uc, "alpha", path)), path)
	}
}

func Test_TheAllScopeTakesOnlyItsOwnToken(t *testing.T) {
	_, uc := setup("k1")
	unset := telemetry.NewTelemetryUsecase("k1", "", catalog{}, directory{}, &upstream{})

	require.Equal(t, apperror.CodeUnauthenticated, apperror.CodeOf(loki(uc, entities.AllTeams, "/loki/api/v1/labels")))
	require.Equal(t, apperror.CodeUnauthenticated, apperror.CodeOf(lokiWith(unset, entities.AllTeams, "", "/loki/api/v1/labels")))
}

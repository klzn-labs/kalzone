package telemetry

import (
	"context"
	"net/url"
	"regexp"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type PrometheusRequest struct {
	Team  string
	Token string
	Path  string
	Form  url.Values
}

type paramKind int

const (
	plainParams paramKind = iota
	queryParam
	matchParams
)

var labelValuesPath = regexp.MustCompile(`^/api/v1/label/[^/]+/values$`)

var prometheusPaths = map[string]paramKind{
	"/api/v1/query": queryParam, "/api/v1/query_range": queryParam, "/api/v1/query_exemplars": queryParam,
	"/api/v1/series": matchParams, "/api/v1/labels": matchParams,
	"/api/v1/metadata": plainParams, "/api/v1/status/buildinfo": plainParams,
}

func (u TelemetryUsecase) Prometheus(ctx context.Context, r PrometheusRequest) (entities.TelemetryAnswer, error) {
	if err := u.authorize(r.Team, r.Token); err != nil {
		return entities.TelemetryAnswer{}, err
	}
	kind, ok := prometheusKind(r.Path)
	if !ok || r.Team == entities.AllTeams {
		return entities.TelemetryAnswer{}, errPathRefused
	}
	s, err := u.teamScope(ctx, r.Team)
	if err != nil {
		return entities.TelemetryAnswer{}, err
	}
	form, err := scopedForm(kind, r.Form, s)
	if err != nil {
		return entities.TelemetryAnswer{}, err
	}
	return u.upstream.Prometheus(ctx, r.Path, form)
}

func prometheusKind(path string) (paramKind, bool) {
	if labelValuesPath.MatchString(path) {
		return matchParams, true
	}
	kind, ok := prometheusPaths[path]
	return kind, ok
}

func scopedForm(kind paramKind, form url.Values, s entities.Scope) (url.Values, error) {
	out := url.Values{}
	for name, values := range form {
		out[name] = append([]string{}, values...)
	}
	switch kind {
	case queryParam:
		return out, scopeQueryParam(out, s)
	case matchParams:
		return out, scopeMatchParams(out, s)
	}
	return out, nil
}

func scopeQueryParam(form url.Values, s entities.Scope) error {
	query, err := entities.ScopeQuery(form.Get("query"), s)
	if err != nil {
		return apperror.Invalid("the query is not PromQL: " + err.Error()).WithField("query")
	}
	form.Set("query", query)
	return nil
}

func scopeMatchParams(form url.Values, s entities.Scope) error {
	if len(form["match[]"]) == 0 {
		form["match[]"] = entities.ScopeDefaultSelectors(s)
		return nil
	}
	for i, selector := range form["match[]"] {
		scoped, err := entities.ScopeSelector(selector, s)
		if err != nil {
			return apperror.Invalid("match[] is not a series selector: " + err.Error()).WithField("match[]")
		}
		form["match[]"][i] = scoped
	}
	return nil
}

func (u TelemetryUsecase) teamScope(ctx context.Context, team string) (entities.Scope, error) {
	teamID, err := u.directory.TeamID(ctx, team)
	if err != nil {
		return entities.Scope{}, err
	}
	services, err := u.catalog.Services(ctx)
	if err != nil {
		return entities.Scope{}, err
	}
	environments, err := u.directory.Environments(ctx)
	return entities.TeamScope(services, teamID, environments), err
}

package telemetry

import (
	"context"
	"strings"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

type LokiRequest struct {
	Team  string
	Token string
	Call  entities.LokiCall
}

var lokiReadPaths = map[string]bool{
	"/loki/api/v1/query": true, "/loki/api/v1/query_range": true, "/loki/api/v1/labels": true,
	"/loki/api/v1/series": true, "/loki/api/v1/patterns": true,
}

var lokiReadPrefixes = []string{"/loki/api/v1/label/", "/loki/api/v1/index/", "/loki/api/v1/detected_"}

func (u TelemetryUsecase) Loki(ctx context.Context, r LokiRequest) (entities.TelemetryAnswer, error) {
	if err := u.authorize(r.Team, r.Token); err != nil {
		return entities.TelemetryAnswer{}, err
	}
	if !lokiReadable(r.Call.Path) {
		return entities.TelemetryAnswer{}, errPathRefused
	}
	tenants, err := u.tenants(ctx, r.Team)
	if err != nil {
		return entities.TelemetryAnswer{}, err
	}
	r.Call.Tenants = tenants
	return u.upstream.Loki(ctx, r.Call)
}

func lokiReadable(path string) bool {
	if lokiReadPaths[path] {
		return true
	}
	for _, prefix := range lokiReadPrefixes {
		if strings.HasPrefix(path, prefix) && !strings.Contains(path, "..") {
			return true
		}
	}
	return false
}

func (u TelemetryUsecase) tenants(ctx context.Context, team string) ([]string, error) {
	if team == entities.AllTeams {
		return u.allTenants(ctx)
	}
	s, err := u.teamScope(ctx, team)
	if err != nil {
		return nil, err
	}
	if tenants := s.Tenants(); len(tenants) > 0 {
		return tenants, nil
	}
	return []string{entities.NoTenant}, nil
}

func (u TelemetryUsecase) allTenants(ctx context.Context) ([]string, error) {
	services, err := u.catalog.Services(ctx)
	if err != nil {
		return nil, err
	}
	environments, err := u.directory.Environments(ctx)
	return entities.AllTenants(services, environments), err
}

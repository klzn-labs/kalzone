package telemetry

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var (
	errNotConfigured = apperror.New(apperror.CodeNotConfigured, "team telemetry is not configured on this platform").
				WithHint("set PLATFORM_TELEMETRY_KEY")
	errBadToken    = apperror.New(apperror.CodeUnauthenticated, "this token does not open this team's telemetry")
	errPathRefused = apperror.Forbidden("this telemetry endpoint is not readable through a team")
)

type Catalog interface {
	Services(ctx context.Context) ([]entities.Service, error)
}

type Directory interface {
	TeamID(ctx context.Context, teamSlug string) (string, error)
	Environments(ctx context.Context) ([]string, error)
}

type Upstream interface {
	Prometheus(ctx context.Context, path string, form url.Values) (entities.TelemetryAnswer, error)
	Loki(ctx context.Context, call entities.LokiCall) (entities.TelemetryAnswer, error)
}

type TelemetryUsecase struct {
	key       []byte
	allToken  string
	catalog   Catalog
	directory Directory
	upstream  Upstream
}

func NewTelemetryUsecase(key, allToken string, catalog Catalog, directory Directory, upstream Upstream) TelemetryUsecase {
	return TelemetryUsecase{key: []byte(key), allToken: allToken, catalog: catalog, directory: directory, upstream: upstream}
}

func (u TelemetryUsecase) Enabled() bool {
	return len(u.key) > 0
}

func (u TelemetryUsecase) Token(team string) string {
	mac := hmac.New(sha256.New, u.key)
	mac.Write([]byte("telemetry:" + team))
	return hex.EncodeToString(mac.Sum(nil))
}

func (u TelemetryUsecase) authorize(team, token string) error {
	if !u.Enabled() {
		return errNotConfigured
	}
	expected := u.expected(team)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(token)) {
		return errBadToken
	}
	return nil
}

func (u TelemetryUsecase) expected(team string) string {
	if team == entities.AllTeams {
		return u.allToken
	}
	return u.Token(team)
}

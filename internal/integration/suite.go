package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/release/repositories/registry"
	"github.com/klzn-labs/kalzone/cmd/controlplane/options"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/internal/fakegrafana"
	"github.com/klzn-labs/kalzone/internal/fakeoidc"
	"github.com/klzn-labs/kalzone/internal/fakepushgateway"
	"github.com/klzn-labs/kalzone/internal/fakeregistry"
	"github.com/klzn-labs/kalzone/internal/faketelemetry"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"
	"gorm.io/gorm"
)

const (
	Audience      = "http://platform.test"
	WebhookSecret = "webhook-secret"
	AgentToken    = "agent-token"
	QueuedToken   = "queued-test-token"
	BuildWorkflow = "klzn-labs/kalzone-actions/.github/workflows/build.yml"
	RegistryToken = "dckr_pat_test"
	TelemetryKey  = "telemetry-key"
)

type Suite struct {
	Server      *httptest.Server
	GitHub      *fakegithub.Server
	OIDC        *fakeoidc.Issuer
	Registry    *fakeregistry.Registry
	Grafana     *fakegrafana.Grafana
	Pushgateway *fakepushgateway.Pushgateway
	Telemetry   *faketelemetry.Store
	Engine      *gin.Engine
	Catalog     *apispec.Catalog
	Drain       *wait.Drain
	db          *gorm.DB
	contract    *contract
	nextID      int64
}

type Option func(*config.Config)

func WithDemo(cfg *config.Config) {
	cfg.Demo.Enabled = true
}

func NewSuite(t *testing.T, tweaks ...Option) *Suite {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testdb.Open(t)
	s := &Suite{GitHub: fakegithub.New(), OIDC: fakeoidc.New(), Registry: fakeregistry.New(), Grafana: fakegrafana.New(), Pushgateway: fakepushgateway.New(), Telemetry: faketelemetry.New(), nextID: 1000}
	app := fxtest.New(t,
		options.Options(),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Decorate(func(config.Config) config.Config { return withOptions(testConfig(t, s), tweaks) }),
		fx.Decorate(func(registry.Resolver) registry.Resolver { return registry.WithBaseURL(s.Registry.URL) }),
		fx.Populate(&s.Engine, &s.db, &s.Catalog, &s.Drain),
	)
	app.RequireStart()
	s.contract = loadContract(t, s.Catalog)
	s.Server = httptest.NewServer(s.Engine)
	t.Cleanup(func() { s.close(app) })
	return s
}

func (s *Suite) close(app *fxtest.App) {
	s.Server.Close()
	s.GitHub.Close()
	s.OIDC.Close()
	s.Registry.Close()
	s.Grafana.Close()
	app.RequireStop()
	s.Pushgateway.Close()
	s.Telemetry.Close()
	testdb.Close(s.db)
}

func withOptions(cfg config.Config, tweaks []Option) config.Config {
	for _, tweak := range tweaks {
		tweak(&cfg)
	}
	return cfg
}

func testConfig(t *testing.T, s *Suite) config.Config {
	cfg := baseConfig(s.GitHub.URL)
	cfg.GitHub.AppID, cfg.GitHub.AppPrivateKey, cfg.GitHub.WebhookSecret = 1, appKey(t), WebhookSecret
	cfg.OIDC = config.OIDC{Issuer: s.OIDC.URL, Audience: Audience}
	cfg.Build.Timeout = time.Hour
	cfg.Build.Workflow = BuildWorkflow
	cfg.Registry = config.Registry{Username: "bot", Token: RegistryToken, AuthURL: s.Registry.URL + "/token"}
	cfg.Deploy.Timeout = time.Hour
	cfg.Agent.Token = AgentToken
	cfg.Queues = config.Queues{GatewayURL: "http://queued:8080", Token: QueuedToken}
	cfg.Grafana = config.Grafana{URL: s.Grafana.URL + "/grafana", AdminUser: fakegrafana.AdminUser, AdminPassword: fakegrafana.AdminPassword, PrometheusUID: "prometheus", LokiUID: "loki", JWTKey: fakegrafana.SigningKey()}
	cfg.Databases = testDatabases(t)
	cfg.Metrics = config.Metrics{PushgatewayURL: s.Pushgateway.URL}
	cfg.Telemetry = config.Telemetry{Key: TelemetryKey, InternalURL: "http://controlplane:8080", PrometheusURL: s.Telemetry.URL, LokiURL: s.Telemetry.URL + "/loki-root"}
	cfg.Health = config.Health{ResponseMS: 300, CPUPercent: 70, DBCPUPercent: 70}
	cfg.Demo = config.Demo{SessionTTL: 2 * time.Hour, PerIPHour: 5}
	return cfg
}

func testDatabases(t *testing.T) config.Databases {
	parsed, err := driver.ParseDSN(testdb.DSN())
	if err != nil {
		t.Fatal(err)
	}
	host, rawPort, err := net.SplitHostPort(parsed.Addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	parsed.DBName = ""
	return config.Databases{AdminDSN: parsed.FormatDSN(), Host: host, Port: port}
}

func appKey(t *testing.T) string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func baseConfig(githubURL string) config.Config {
	return config.Config{
		HTTP:           config.HTTP{Addr: ":0"},
		BaseURL:        "http://platform.test",
		DB:             testdb.Config().DB,
		GitHub:         config.GitHub{ClientID: "id", ClientSecret: "secret", OAuthURL: githubURL, APIURL: githubURL},
		Admins:         []string{"root"},
		Secrets:        config.Secrets{Key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=", KeyID: "k1"},
		AllowedDomains: []string{"kalz.one"},
		Session:        config.Session{TTL: time.Hour, RenewWithin: 10 * time.Minute},
	}
}

type Route struct {
	Method string
	Path   string
}

func (s *Suite) Routes() []Route {
	routes := []Route{}
	for _, r := range s.Engine.Routes() {
		routes = append(routes, Route{Method: r.Method, Path: r.Path})
	}
	return routes
}

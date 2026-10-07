package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/release/repositories/registry"
	"github.com/adrianozp/platform/cmd/controlplane/options"
	"github.com/adrianozp/platform/internal/fakegithub"
	"github.com/adrianozp/platform/internal/fakeoidc"
	"github.com/adrianozp/platform/internal/fakeregistry"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"
)

const (
	Audience      = "http://platform.test"
	WebhookSecret = "webhook-secret"
)

type Suite struct {
	Server   *httptest.Server
	GitHub   *fakegithub.Server
	OIDC     *fakeoidc.Issuer
	Registry *fakeregistry.Registry
	Engine   *gin.Engine
	nextID   int64
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testdb.Open(t)
	s := &Suite{GitHub: fakegithub.New(), OIDC: fakeoidc.New(), Registry: fakeregistry.New(), nextID: 1000}
	app := fxtest.New(t,
		options.Options(),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Decorate(func(config.Config) config.Config { return testConfig(t, s) }),
		fx.Decorate(func(registry.Resolver) registry.Resolver { return registry.WithBaseURL(s.Registry.URL) }),
		fx.Populate(&s.Engine),
	)
	app.RequireStart()
	s.Server = httptest.NewServer(s.Engine)
	t.Cleanup(func() { s.close(app) })
	return s
}

func (s *Suite) close(app *fxtest.App) {
	s.Server.Close()
	s.GitHub.Close()
	s.OIDC.Close()
	s.Registry.Close()
	app.RequireStop()
}

func testConfig(t *testing.T, s *Suite) config.Config {
	cfg := baseConfig(s.GitHub.URL)
	cfg.GitHub.AppID, cfg.GitHub.AppPrivateKey, cfg.GitHub.WebhookSecret = 1, appKey(t), WebhookSecret
	cfg.OIDC = config.OIDC{Issuer: s.OIDC.URL, Audience: Audience}
	cfg.Build.Timeout = time.Hour
	return cfg
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

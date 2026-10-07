package integration

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adrianozp/platform/cmd/controlplane/options"
	"github.com/adrianozp/platform/internal/fakegithub"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"
)

type Suite struct {
	Server *httptest.Server
	GitHub *fakegithub.Server
	Engine *gin.Engine
	nextID int64
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testdb.Open(t)
	github := fakegithub.New()
	var engine *gin.Engine
	app := fxtest.New(t,
		options.Options(),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Decorate(func(config.Config) config.Config { return testConfig(github.URL) }),
		fx.Populate(&engine),
	)
	app.RequireStart()
	server := httptest.NewServer(engine)
	t.Cleanup(func() {
		server.Close()
		github.Close()
		app.RequireStop()
	})
	return &Suite{Server: server, GitHub: github, Engine: engine, nextID: 1000}
}

func testConfig(githubURL string) config.Config {
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

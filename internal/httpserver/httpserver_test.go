package httpserver_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

func Test_Run_ClosesTheDrainWhenTheServerStops(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lc := fxtest.NewLifecycle(t)
	drain := wait.NewDrain()
	httpserver.Run(lc, config.Config{HTTP: config.HTTP{Addr: "127.0.0.1:0"}}, gin.New(), drain)
	lc.RequireStart().RequireStop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	never := func(context.Context) (bool, error) { return false, nil }

	_, err := wait.Until(ctx, wait.NewWaiter(drain, nil), time.Minute, false, never, func(done bool) bool { return done })

	require.NoError(t, err)
	require.NoError(t, ctx.Err())
}

func Test_TheRequestLogHidesLinkTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logged bytes.Buffer
	previous := gin.DefaultWriter
	gin.DefaultWriter = &logged
	t.Cleanup(func() { gin.DefaultWriter = previous })
	engine := httpserver.NewEngine()

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/join/s3cret-token?x=1", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/login", nil))

	require.NotContains(t, logged.String(), "s3cret-token")
	require.Contains(t, logged.String(), "/join/[redacted]")
	require.Contains(t, logged.String(), `"/login"`)
}

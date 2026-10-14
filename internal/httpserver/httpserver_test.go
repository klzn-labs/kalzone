package httpserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/httpserver/wait"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
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

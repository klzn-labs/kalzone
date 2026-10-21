package platform_test

import (
	"testing"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/web/handlers/platform"
	"github.com/stretchr/testify/require"
)

func Test_GraphURLOpensTheDashboardOnTenMinutesAroundTheDeploy(t *testing.T) {
	at := time.Date(2026, 10, 21, 16, 9, 31, 0, time.UTC)

	got := platform.GraphURL("controlplane", deploy.PlatformDeploy{At: at})

	require.Equal(t, "/grafana/d/kalzone-http?var-service=controlplane&time=1792598971000&time.window=600000", got)
}

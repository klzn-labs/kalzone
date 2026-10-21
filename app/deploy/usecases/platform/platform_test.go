package platform_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/platform"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func Test_Component_ReturnsTheLatestReport(t *testing.T) {
	store := fakestore.New()
	store.Platforms["controlplane"] = entities.PlatformComponent{Name: "controlplane", Live: entities.PlatformRelease{Version: "abc"}}

	got, err := platform.NewPlatformUsecase(store).Component(ctx, "controlplane")

	require.NoError(t, err)
	require.Equal(t, "abc", got.Live.Version)
}

func Test_Component_NotReportedYetIsNotFound(t *testing.T) {
	_, err := platform.NewPlatformUsecase(fakestore.New()).Component(ctx, "controlplane")

	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
}

func Test_Component_RefusesUnknownNames(t *testing.T) {
	_, err := platform.NewPlatformUsecase(fakestore.New()).Component(ctx, "api")

	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
}

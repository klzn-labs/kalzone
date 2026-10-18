package configs_test

import (
	"fmt"
	"testing"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_SetSecret_SealsBoundToServiceEnvironmentAndName(t *testing.T) {
	f := newFixture(t)

	rev, err := f.usecase.SetSecret(ctx, ana, "api", "prod", "DB_PASSWORD", "hunter2")

	require.NoError(t, err)
	require.Equal(t, "s1|prod|DB_PASSWORD", string(f.sealer.additionalData[0]))
	require.Equal(t, f.store.versions[0].ID, rev.Secrets["DB_PASSWORD"])
	require.Equal(t, "set secret DB_PASSWORD", rev.Message)
}

func Test_SetSecret_OldRevisionsKeepTheirSecretVersion(t *testing.T) {
	f := newFixture(t)
	first, _ := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "one")

	second, err := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "two")

	require.NoError(t, err)
	require.NotEqual(t, first.Secrets["TOKEN"], second.Secrets["TOKEN"])
	require.Equal(t, first.Secrets["TOKEN"], f.store.revisions[0].Secrets["TOKEN"])
}

func Test_SetSecret_ValueNeverReachesEventsOrRevisions(t *testing.T) {
	f := newFixture(t)

	rev, err := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "hunter2")

	require.NoError(t, err)
	require.NotContains(t, fmt.Sprint(f.events.events), "hunter2")
	require.NotContains(t, fmt.Sprint(rev), "hunter2")
}

func Test_SetSecret_Validation(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"PLAIN": "x"}})
	require.NoError(t, err)

	_, badName := f.usecase.SetSecret(ctx, ana, "api", "dev", "token", "x")
	_, empty := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "")
	_, clash := f.usecase.SetSecret(ctx, ana, "api", "dev", "PLAIN", "x")

	require.ErrorIs(t, badName, apperror.Invalid(""))
	require.ErrorIs(t, empty, apperror.Invalid(""))
	require.ErrorIs(t, clash, apperror.Invalid(""))
}

func Test_UnsetSecret_RemovesItOrIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "x")
	require.NoError(t, err)

	rev, err := f.usecase.UnsetSecret(ctx, ana, "api", "dev", "TOKEN")
	_, missing := f.usecase.UnsetSecret(ctx, ana, "api", "dev", "TOKEN")

	require.NoError(t, err)
	require.Empty(t, rev.Secrets)
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_Secrets_ListsCurrentNamesWithoutValues(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "hunter2")
	require.NoError(t, err)

	infos, err := f.usecase.Secrets(ctx, "api", "dev")

	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Equal(t, "TOKEN", infos[0].Name)
	require.Equal(t, "ana", infos[0].SetBy)
}

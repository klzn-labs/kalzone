package configs_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/stretchr/testify/require"
)

func Test_Resolve_OpensTheSecretsOfThatExactRevision(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "one")
	require.NoError(t, err)
	_, err = f.usecase.SetSecret(ctx, ana, "api", "dev", "TOKEN", "two")
	require.NoError(t, err)

	first, err := f.usecase.Resolve(ctx, "s1", "dev", 1)
	require.NoError(t, err)
	second, _ := f.usecase.Resolve(ctx, "s1", "dev", 2)
	defaults, _ := f.usecase.Resolve(ctx, "s1", "prod", 0)

	require.Equal(t, map[string]string{"TOKEN": "one"}, first.Secrets)
	require.Equal(t, map[string]string{"TOKEN": "two"}, second.Secrets)
	require.Equal(t, entities.DefaultSpec(), defaults.Revision.Spec)
	require.Empty(t, defaults.Secrets)
}

func Test_CurrentNumber(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"A": "1"}})
	require.NoError(t, err)

	dev, _ := f.usecase.CurrentNumber(ctx, "api", "dev")
	prod, _ := f.usecase.CurrentNumber(ctx, "api", "prod")

	require.Equal(t, 1, dev)
	require.Equal(t, 0, prod)
}

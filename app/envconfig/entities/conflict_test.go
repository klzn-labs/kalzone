package entities_test

import (
	"testing"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/stretchr/testify/require"
)

func revisionWith(env map[string]string, secrets map[string]string) entities.Revision {
	r := entities.Initial("svc", "prod")
	for k, v := range env {
		r.Env[k] = v
	}
	for k, v := range secrets {
		r.Secrets[k] = v
	}
	return r
}

func Test_ChangeSet_KeysNameEveryTouchedKeyOnce(t *testing.T) {
	spec := entities.DefaultSpec()
	cs := entities.ChangeSet{
		Spec: &spec, SetEnv: map[string]string{"B": "1", "A": "2"}, UnsetEnv: []string{"A"},
		SetSecrets: map[string]string{"TOKEN": "v2"}, UnsetSecrets: []string{"OLD"},
	}

	require.Equal(t, []string{"env.A", "env.B", "secret.OLD", "secret.TOKEN", "spec"}, cs.Keys())
}

func Test_ApplyTo_SetsAndUnsetsSecretVersions(t *testing.T) {
	base := revisionWith(nil, map[string]string{"OLD": "v0"})
	cs := entities.ChangeSet{SetSecrets: map[string]string{"TOKEN": "v1"}, UnsetSecrets: []string{"OLD"}}

	next := cs.ApplyTo(base.Next())

	require.Equal(t, map[string]string{"TOKEN": "v1"}, next.Secrets)
}

func Test_Conflicts_OnlyKeysTheRequestTouchesAndThatMovedSinceItsBase(t *testing.T) {
	base := revisionWith(map[string]string{"A": "1", "B": "1"}, nil)
	current := revisionWith(map[string]string{"A": "9", "B": "1", "C": "1"}, nil)
	current.Spec.Replicas = 3

	clean := entities.Conflicts(entities.ChangeSet{SetEnv: map[string]string{"B": "2"}}, base, current)
	clash := entities.Conflicts(entities.ChangeSet{SetEnv: map[string]string{"A": "2"}, UnsetEnv: []string{"C"}}, base, current)
	spec := entities.DefaultSpec()
	wholeSpec := entities.Conflicts(entities.ChangeSet{Spec: &spec}, base, current)

	require.Empty(t, clean)
	require.Equal(t, []string{"env.A", "env.C"}, clash)
	require.Equal(t, []string{"spec"}, wholeSpec)
}

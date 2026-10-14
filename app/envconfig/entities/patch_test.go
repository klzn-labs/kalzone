package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func decoded(t *testing.T, raw string) any {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal([]byte(raw), &value))
	return value
}

func Test_MergePatch_FollowsRFC7396(t *testing.T) {
	cases := [][3]string{
		{`{"a":"b"}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"b"}`, `{"a":null}`, `{}`},
		{`{"a":{"b":"c"}}`, `{"a":{"b":"d","c":null}}`, `{"a":{"b":"d"}}`},
		{`{"a":["b"]}`, `{"a":["c","d"]}`, `{"a":["c","d"]}`},
		{`["a"]`, `{"a":"b"}`, `{"a":"b"}`},
		{`{"a":"b"}`, `["c"]`, `["c"]`},
		{`{}`, `{"a":{"bb":{"ccc":null}}}`, `{"a":{"bb":{}}}`},
	}
	for _, c := range cases {
		merged := entities.MergePatch(decoded(t, c[0]), decoded(t, c[1]))
		require.Equal(t, decoded(t, c[2]), merged, c[1])
	}
}

func Test_PatchChangeSet_KeepsWhatThePatchDoesNotName(t *testing.T) {
	current := revisionWith(map[string]string{"LOG_LEVEL": "info", "OLD_FLAG": "1", "KEEP": "x"}, nil)
	current.Spec.Aliases = []string{"www.kalz.one"}
	patch := `{"spec":{"replicas":2,"aliases":null},"env":{"LOG_LEVEL":"debug","OLD_FLAG":null,"KEEP":"x"},"message":"more replicas"}`

	cs, err := entities.PatchChangeSet(current, []byte(patch))

	require.NoError(t, err)
	want := entities.DefaultSpec()
	want.Replicas = 2
	require.Equal(t, &want, cs.Spec)
	require.Equal(t, map[string]string{"LOG_LEVEL": "debug"}, cs.SetEnv)
	require.Equal(t, []string{"OLD_FLAG"}, cs.UnsetEnv)
	require.Equal(t, "more replicas", cs.Message)
}

func Test_PatchChangeSet_LeavesTheSpecAloneWhenItIsNotSent(t *testing.T) {
	cs, err := entities.PatchChangeSet(revisionWith(nil, nil), []byte(`{"env":{"A":"1"}}`))

	require.NoError(t, err)
	require.Nil(t, cs.Spec)
	require.Equal(t, map[string]string{"A": "1"}, cs.SetEnv)
}

func Test_PatchChangeSet_RefusesWhatIsNotAConfigChange(t *testing.T) {
	cases := map[string]string{
		`[1]`:                         "",
		`{"spec":null}`:               "spec",
		`{"env":null}`:                "env",
		`{"spec":{"replica":2}}`:      "spec.replica",
		`{"secrets":{"A":"1"}}`:       "secrets",
		`{"env":{"A":1}}`:             "env",
		`{"spec":{"replicas":"two"}}`: "spec.replicas",
		`{"message":3}`:               "message",
		`{"ENV":{"B":"2"}}`:           "ENV",
		`{"Spec":{"replicas":2}}`:     "Spec",
		`{"Spec":null}`:               "Spec",
		`{"spec":{"Replicas":3}}`:     "spec.Replicas",
		`{"env":"x"}`:                 "env",
		`{"spec":3}`:                  "spec",
	}
	for patch, field := range cases {
		_, err := entities.PatchChangeSet(revisionWith(nil, nil), []byte(patch))
		require.ErrorIs(t, err, apperror.Invalid(""), patch)
		require.Equal(t, field, err.(*apperror.AppError).Field, patch)
	}
}

func Test_PatchChangeSet_RefusesKeysThatDifferOnlyInCase(t *testing.T) {
	current := revisionWith(map[string]string{"A": "1"}, nil)

	_, mixed := entities.PatchChangeSet(current, []byte(`{"env":{"B":"2"},"ENV":{"A":null}}`))
	_, nested := entities.PatchChangeSet(current, []byte(`{"spec":{"replicas":2,"Replicas":3}}`))

	require.Equal(t, "ENV", mixed.(*apperror.AppError).Field)
	require.Contains(t, mixed.(*apperror.AppError).Hint, "spec, env, message")
	require.Equal(t, "spec.Replicas", nested.(*apperror.AppError).Field)
	require.Contains(t, nested.(*apperror.AppError).Hint, "replicas")
}

func Test_PatchChangeSet_SaysEnvMustBeAnObject(t *testing.T) {
	_, err := entities.PatchChangeSet(revisionWith(nil, nil), []byte(`{"env":"x"}`))

	require.Equal(t, "env must be an object", err.(*apperror.AppError).Message)
}

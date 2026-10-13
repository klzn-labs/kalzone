package apperror_test

import (
	"encoding/json"
	"testing"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_HelpersCopyInsteadOfChangingTheSharedError(t *testing.T) {
	shared := apperror.Invalid("bad")

	helped := shared.WithHint("send x").WithField("x").WithDetails(map[string]any{"accepted": []string{"x"}})

	require.Empty(t, shared.Hint)
	require.Empty(t, shared.Field)
	require.Nil(t, shared.Details)
	require.Equal(t, "send x", helped.Hint)
	require.Equal(t, "x", helped.Field)
	require.ErrorIs(t, helped, shared)
}

func Test_EmptyHelpFieldsStayOutOfTheJSON(t *testing.T) {
	plain, err := json.Marshal(apperror.NotFound("gone"))
	require.NoError(t, err)
	helped, err := json.Marshal(apperror.NotFound("gone").WithHint("list it").WithField("id"))
	require.NoError(t, err)

	require.JSONEq(t, `{"code":"err_not_found","message":"gone"}`, string(plain))
	require.JSONEq(t, `{"code":"err_not_found","message":"gone","hint":"list it","field":"id"}`, string(helped))
}

func Test_ScopeAndTeamErrorsSayHowToFixThem(t *testing.T) {
	require.Contains(t, apperror.MissingScope("deploy").Hint, "deploy scope")
	require.Contains(t, apperror.TeamNotGranted("t1").Hint, "granted this team")
}

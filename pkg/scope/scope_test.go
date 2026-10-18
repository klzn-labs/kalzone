package scope_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func Test_Parse_KeepsKnownScopesOnceInOrder(t *testing.T) {
	got, err := scope.Parse([]string{"deploy", "read", "deploy"})

	require.NoError(t, err)
	require.Equal(t, []scope.Scope{scope.Deploy, scope.Read}, got)
}

func Test_Parse_KnowsTheLogsScope(t *testing.T) {
	got, err := scope.Parse([]string{"logs"})

	require.NoError(t, err)
	require.Equal(t, []scope.Scope{scope.Logs}, got)
}

func Test_Parse_RejectsUnknownScopes(t *testing.T) {
	_, err := scope.Parse([]string{"read", "root"})

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Contains(t, err.Error(), `"root"`)
}

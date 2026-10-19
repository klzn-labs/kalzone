package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_SchemaFor(t *testing.T) {
	require.Equal(t, "my_app_dev", entities.SchemaFor("my-app", "dev", false))
	require.Equal(t, "my_app", entities.SchemaFor("my-app", "prod", true))
	require.Equal(t, "x_qa", entities.SchemaFor("x", "qa", false))
}

func Test_UsernameFor_PrefixesTheSchema(t *testing.T) {
	user, err := entities.UsernameFor("my_app")

	require.NoError(t, err)
	require.Equal(t, "kz_my_app", user)
}

func Test_UsernameFor_RefusesOver32(t *testing.T) {
	_, err := entities.UsernameFor(strings.Repeat("a", 30))

	require.ErrorIs(t, err, apperror.Invalid(""))
}

func Test_ValidateName(t *testing.T) {
	require.NoError(t, entities.ValidateName("ab"))
	require.Error(t, entities.ValidateName("a"))
	require.Error(t, entities.ValidateName(strings.Repeat("a", 21)))
	require.Error(t, entities.ValidateName("Ab"))
}

func Test_ValidateMaxConnections(t *testing.T) {
	require.Error(t, entities.ValidateMaxConnections(0))
	require.NoError(t, entities.ValidateMaxConnections(1))
	require.NoError(t, entities.ValidateMaxConnections(30))
	require.Error(t, entities.ValidateMaxConnections(31))
}

func Test_Reserved(t *testing.T) {
	for _, name := range []string{"mysql", "sys", "information_schema", "performance_schema", "platform"} {
		require.True(t, entities.Reserved(name), name)
	}
	require.False(t, entities.Reserved("orders"))
}

func Test_Attachment_ActiveUntilDetached(t *testing.T) {
	now := time.Now()
	require.True(t, entities.Attachment{}.Active())
	require.False(t, entities.Attachment{DetachedAt: &now}.Active())
}

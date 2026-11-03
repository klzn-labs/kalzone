package entities_test

import (
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_PasswordRoundTrip(t *testing.T) {
	first, err := entities.HashPassword("correct horse battery")
	require.NoError(t, err)
	second, err := entities.HashPassword("correct horse battery")
	require.NoError(t, err)
	require.True(t, entities.VerifyPassword(first, "correct horse battery"))
	require.False(t, entities.VerifyPassword(first, "another password!"))
	require.NotEqual(t, first, second)
	require.True(t, strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=2,p=1$"))
}

func Test_PasswordRuleBounds(t *testing.T) {
	require.ErrorIs(t, entities.CheckPassword(strings.Repeat("a", 11)), apperror.Invalid(""))
	require.NoError(t, entities.CheckPassword(strings.Repeat("a", 12)))
	require.NoError(t, entities.CheckPassword(strings.Repeat("a", 128)))
	require.ErrorIs(t, entities.CheckPassword(strings.Repeat("a", 129)), apperror.Invalid(""))
}

func Test_VerifyPasswordRefusesMalformedHash(t *testing.T) {
	require.False(t, entities.VerifyPassword("", "whatever password"))
	require.False(t, entities.VerifyPassword("$argon2id$garbage", "whatever password"))
	require.False(t, entities.VerifyPassword(entities.DummyHash, "whatever password"))
}

func Test_NormalizeLogin(t *testing.T) {
	login, err := entities.NormalizeLogin(" Ana ")
	require.NoError(t, err)
	require.Equal(t, "ana", login)
	for _, bad := range []string{"a.b", "", strings.Repeat("a", 40)} {
		_, err := entities.NormalizeLogin(bad)
		require.ErrorIs(t, err, apperror.Invalid(""), bad)
	}
}

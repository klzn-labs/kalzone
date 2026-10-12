package entities_test

import (
	"strings"
	"testing"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/stretchr/testify/require"
)

func Test_NewTokenSecret_IsPrefixedBase62AndHashed(t *testing.T) {
	secret, err := entities.NewTokenSecret()

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(secret.Raw, "plat_"))
	require.Len(t, secret.Raw, len("plat_")+43)
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, secret.Raw)
	require.Equal(t, secret.Raw[5:13], secret.Prefix)
	require.Equal(t, entities.HashToken(secret.Raw), secret.Hash)
	require.True(t, entities.LooksLikeToken(secret.Raw))
}

func Test_LooksLikeToken_RejectsOtherStrings(t *testing.T) {
	for _, raw := range []string{"", "plat_", "plat_short", strings.Repeat("a", 64), "PLAT_" + strings.Repeat("a", 43)} {
		require.False(t, entities.LooksLikeToken(raw), raw)
	}
}

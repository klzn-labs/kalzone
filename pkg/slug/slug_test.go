package slug_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/pkg/slug"
	"github.com/stretchr/testify/require"
)

func Test_Validate(t *testing.T) {
	valid := []string{"api", "places-api", "a1b", "web2"}
	invalid := []string{"", "ab", "Api", "1api", "api-", "a--b", "api_v2", "a.b", "abcdefghijabcdefghijabcdefghijabcdefghijk"}
	for _, v := range valid {
		require.NoError(t, slug.Validate("service", v), v)
	}
	for _, v := range invalid {
		require.Error(t, slug.Validate("service", v), v)
	}
}

func Test_ValidateName(t *testing.T) {
	require.NoError(t, slug.ValidateName("team", "Places"))
	require.Error(t, slug.ValidateName("team", "   "))
}

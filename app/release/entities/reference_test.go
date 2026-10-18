package entities_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ParseReference_RecognisesEveryForm(t *testing.T) {
	cases := map[string]entities.Reference{
		"0b3c5d1e-2f4a-4b6c-8d9e-0a1b2c3d4e5f": {Kind: entities.ByID, Raw: "0b3c5d1e-2f4a-4b6c-8d9e-0a1b2c3d4e5f"},
		"latest":                               {Kind: entities.ByLatest, Raw: "latest"},
		"r42":                                  {Kind: entities.ByNumber, Raw: "r42", Number: 42},
		"sha:ABC1234":                          {Kind: entities.ByCommit, Raw: "sha:ABC1234", Prefix: "abc1234"},
		"1.4.0":                                {Kind: entities.ByTag, Raw: "1.4.0"},
		"r1-0123456":                           {Kind: entities.ByTag, Raw: "r1-0123456"},
	}
	for raw, want := range cases {
		got, err := entities.ParseReference(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got, raw)
	}
}

func Test_ParseReference_RefusesWhatNamesNoRelease(t *testing.T) {
	for _, raw := range []string{"", "sha:abc12", "sha:xyz1234", "-tag", "two words"} {
		_, err := entities.ParseReference(raw)
		require.ErrorIs(t, err, apperror.Invalid(""), raw)
	}
}

func Test_ParseReference_RefusesNonHexCommitPrefixWithAHint(t *testing.T) {
	for _, raw := range []string{"sha:xyz1234", "sha:abc12%%", "sha:abc_123", "sha:abc123g", "sha:abc1234'--"} {
		_, err := entities.ParseReference(raw)
		require.ErrorIs(t, err, apperror.Invalid(""), raw)
		require.NotEmpty(t, err.(*apperror.AppError).Hint, raw)
	}
}

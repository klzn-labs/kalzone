package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func valid() entities.Cron {
	return entities.Cron{Name: "nightly-import", Schedule: "0 3 * * *", Path: "/hooks/import", Timeout: time.Minute}
}

func Test_ValidateName(t *testing.T) {
	require.NoError(t, entities.ValidateName("ab"))
	require.NoError(t, entities.ValidateName("a"+strings.Repeat("b", 62)))
	require.Error(t, entities.ValidateName("a"))
	require.Error(t, entities.ValidateName("a"+strings.Repeat("b", 63)))
	require.Error(t, entities.ValidateName("Nightly"))
	require.Error(t, entities.ValidateName("1st"))
}

func Test_ValidatePath(t *testing.T) {
	require.NoError(t, entities.ValidatePath("/"))
	require.NoError(t, entities.ValidatePath("/"+strings.Repeat("a", 254)))
	require.Error(t, entities.ValidatePath("hooks"))
	require.Error(t, entities.ValidatePath("/"+strings.Repeat("a", 255)))
}

func Test_ValidateTimeout(t *testing.T) {
	require.Error(t, entities.ValidateTimeout(999*time.Millisecond))
	require.NoError(t, entities.ValidateTimeout(time.Second))
	require.NoError(t, entities.ValidateTimeout(15*time.Minute))
	require.Error(t, entities.ValidateTimeout(15*time.Minute+time.Millisecond))
}

func Test_NormalizePayload_EmptyAndNullMeanNone(t *testing.T) {
	for _, raw := range []string{"", "  ", "null"} {
		got, err := entities.NormalizePayload([]byte(raw))
		require.NoError(t, err)
		require.Nil(t, got, raw)
	}
}

func Test_NormalizePayload_KeepsValidJSONUpTo64KiB(t *testing.T) {
	limit := `"` + strings.Repeat("a", entities.MaxPayloadBytes-2) + `"`

	kept, err := entities.NormalizePayload([]byte(` {"full":true} `))
	atLimit, limitErr := entities.NormalizePayload([]byte(limit))
	_, paddedErr := entities.NormalizePayload([]byte(limit + " "))
	_, tooBig := entities.NormalizePayload([]byte(`"` + strings.Repeat("a", entities.MaxPayloadBytes-1) + `"`))
	_, broken := entities.NormalizePayload([]byte(`{"full":`))

	require.NoError(t, err)
	require.Equal(t, `{"full":true}`, string(kept))
	require.NoError(t, limitErr)
	require.Len(t, atLimit, entities.MaxPayloadBytes)
	require.NoError(t, paddedErr)
	require.ErrorIs(t, tooBig, apperror.Invalid(""))
	require.ErrorIs(t, broken, apperror.Invalid(""))
}

func Test_Validate_ChecksEveryField(t *testing.T) {
	badSchedule, badPath, badTimeout := valid(), valid(), valid()
	badSchedule.Schedule = "@every 1m"
	badPath.Path = "hooks"
	badTimeout.Timeout = 0

	require.NoError(t, valid().Validate())
	for _, c := range []entities.Cron{badSchedule, badPath, badTimeout} {
		require.ErrorIs(t, c.Validate(), apperror.Invalid(""))
	}
}

func Test_Body_IsAnEmptyObjectWithoutPayload(t *testing.T) {
	withPayload := valid()
	withPayload.Payload = []byte(`{"a":1}`)

	require.Equal(t, "{}", string(valid().Body()))
	require.Equal(t, `{"a":1}`, string(withPayload.Body()))
}

package signing_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/signing"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func Test_Sign_VerifyRoundTrip(t *testing.T) {
	header := signing.Sign("key", now, []byte(`{"a":1}`))

	require.Regexp(t, `^t=\d+,v1=[0-9a-f]{64}$`, header)
	require.NoError(t, signing.Verify(header, []string{"key"}, []byte(`{"a":1}`), now.Add(time.Minute), 5*time.Minute))
}

func Test_Verify_RejectsTamperedExpiredAndUnknownKey(t *testing.T) {
	header := signing.Sign("key", now, []byte("body"))

	require.Error(t, signing.Verify(header, []string{"key"}, []byte("other"), now, 5*time.Minute))
	require.Error(t, signing.Verify(header, []string{"key"}, []byte("body"), now.Add(6*time.Minute), 5*time.Minute))
	require.Error(t, signing.Verify(header, []string{"nope"}, []byte("body"), now, 5*time.Minute))
	require.Error(t, signing.Verify("garbage", []string{"key"}, []byte("body"), now, 5*time.Minute))
}

func Test_Verify_AcceptsThePreviousKey(t *testing.T) {
	header := signing.Sign("old", now, []byte("body"))

	require.NoError(t, signing.Verify(header, []string{"new", "old"}, []byte("body"), now, 5*time.Minute))
}

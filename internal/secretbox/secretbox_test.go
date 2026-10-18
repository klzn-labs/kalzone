package secretbox_test

import (
	"bytes"
	"testing"

	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/stretchr/testify/require"
)

func newBox(t *testing.T, keyID string, fill byte) *secretbox.Box {
	t.Helper()
	box, err := secretbox.NewWithKey(keyID, bytes.Repeat([]byte{fill}, 32))
	require.NoError(t, err)
	return box
}

func Test_SealThenOpenRoundTrips(t *testing.T) {
	box := newBox(t, "k1", 7)
	ciphertext, nonce, keyID, err := box.Seal([]byte("hunter2"), []byte("svc|dev|DB_PASSWORD"))
	require.NoError(t, err)

	plaintext, err := box.Open(ciphertext, nonce, []byte("svc|dev|DB_PASSWORD"), keyID)

	require.NoError(t, err)
	require.Equal(t, "hunter2", string(plaintext))
	require.NotContains(t, string(ciphertext), "hunter2")
}

func Test_CiphertextBoundToItsServiceEnvironmentAndName(t *testing.T) {
	box := newBox(t, "k1", 7)
	ciphertext, nonce, keyID, err := box.Seal([]byte("hunter2"), []byte("svc|dev|DB_PASSWORD"))
	require.NoError(t, err)

	for _, moved := range []string{"other|dev|DB_PASSWORD", "svc|prod|DB_PASSWORD", "svc|dev|API_KEY"} {
		_, err := box.Open(ciphertext, nonce, []byte(moved), keyID)
		require.Error(t, err, moved)
	}
}

func Test_OpenRefusesAnotherKey(t *testing.T) {
	ciphertext, nonce, _, err := newBox(t, "k1", 7).Seal([]byte("x"), nil)
	require.NoError(t, err)

	_, unknownErr := newBox(t, "k1", 7).Open(ciphertext, nonce, nil, "k2")
	_, wrongKeyErr := newBox(t, "k1", 8).Open(ciphertext, nonce, nil, "k1")

	require.ErrorIs(t, unknownErr, secretbox.ErrUnknownKey)
	require.Error(t, wrongKeyErr)
}

func Test_NonceIsFreshEachTime(t *testing.T) {
	box := newBox(t, "k1", 7)
	_, first, _, _ := box.Seal([]byte("x"), nil)
	_, second, _, _ := box.Seal([]byte("x"), nil)

	require.NotEqual(t, first, second)
}

func Test_KeyMustBe32Bytes(t *testing.T) {
	_, err := secretbox.NewWithKey("k1", []byte("short"))

	require.ErrorIs(t, err, secretbox.ErrKeySize)
}

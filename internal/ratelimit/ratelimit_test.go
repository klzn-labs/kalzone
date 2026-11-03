package ratelimit_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/internal/ratelimit"
	"github.com/stretchr/testify/require"
)

func Test_Allow_CountsEachKeyOnItsOwn(t *testing.T) {
	l := ratelimit.PerHour(2)

	first, second, third := l.Allow("a"), l.Allow("a"), l.Allow("a")
	other := l.Allow("b")

	require.True(t, first)
	require.True(t, second)
	require.False(t, third)
	require.True(t, other)
}

func Test_TakeConsumesAndRefundGivesBackUpToTheBurst(t *testing.T) {
	l := ratelimit.PerHour(2)

	require.True(t, l.Take("a"))
	require.True(t, l.Take("a"))
	require.False(t, l.Take("a"))
	l.Refund("a")
	require.True(t, l.Take("a"))
	l.Refund("b")
	l.Refund("b")
	require.True(t, l.Take("b"))
	require.True(t, l.Take("b"))
	require.False(t, l.Take("b"))
}

func Test_ClearForgetsTheKeyAndItsSuffixedKeys(t *testing.T) {
	l := ratelimit.PerHour(1)
	l.Take("user:ana|1.1.1.1")
	l.Take("user:ana")
	l.Take("user:anabel|1.1.1.1")

	l.Clear("user:ana")

	require.True(t, l.Take("user:ana|1.1.1.1"))
	require.True(t, l.Take("user:ana"))
	require.False(t, l.Take("user:anabel|1.1.1.1"))
}

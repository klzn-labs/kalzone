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

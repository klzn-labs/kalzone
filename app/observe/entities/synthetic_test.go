package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_SyntheticLines_EndAtTheGivenTimeAndRespectTheLimit(t *testing.T) {
	end := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

	lines := entities.SyntheticLines("demo-shop-api-dev", end, 10)

	require.Len(t, lines, 10)
	require.True(t, lines[9].At.Before(end))
	require.Contains(t, lines[0].Line, "GET /api/products")
}

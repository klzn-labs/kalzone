package tests

import (
	"net/url"
	"testing"

	"github.com/adrianozp/platform/internal/fakegithub"
	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

func fakeUser() fakegithub.User {
	return fakegithub.User{ID: 1, Login: "someone"}
}

func stateOf(t *testing.T, location string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	return parsed.Query().Get("state")
}

func setupSuiteOnly(t *testing.T) *integration.Suite {
	t.Helper()
	return integration.NewSuite(t)
}

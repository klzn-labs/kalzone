package tests

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/internal/integration"
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

func decode(t *testing.T, body string, out any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(body), out))
}

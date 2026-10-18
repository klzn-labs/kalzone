package registry_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klzn-labs/kalzone/app/onboarding/repositories/registry"
	"github.com/stretchr/testify/require"
)

func authServer(t *testing.T, actions []string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		require.Equal(t, "bot", user)
		require.Equal(t, "dckr_pat_x", pass)
		require.Equal(t, "repository:o/hello:push,pull", r.URL.Query().Get("scope"))
		claims, _ := json.Marshal(map[string]any{"access": []map[string]any{{"type": "repository", "name": "o/hello", "actions": actions}}})
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"})
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func Test_CanPush_ReadsThePushActionFromTheRegistryToken(t *testing.T) {
	pusher := registry.New(authServer(t, []string{"pull", "push"}), "bot", "dckr_pat_x")
	reader := registry.New(authServer(t, []string{"pull"}), "bot", "dckr_pat_x")

	push, err := pusher.CanPush(context.Background(), "docker.io/o/hello")
	readOnly, readErr := reader.CanPush(context.Background(), "o/hello")

	require.NoError(t, err)
	require.NoError(t, readErr)
	require.True(t, push)
	require.False(t, readOnly)
}

func Test_CanPush_NeedsCredentialsAndDockerHub(t *testing.T) {
	_, unset := registry.New("http://unused", "", "").CanPush(context.Background(), "docker.io/o/hello")
	_, elsewhere := registry.New("http://unused", "bot", "x").CanPush(context.Background(), "ghcr.io/o/hello")

	require.ErrorContains(t, unset, "no registry credentials")
	require.ErrorContains(t, elsewhere, "Docker Hub")
}

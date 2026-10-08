package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

const digest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func fakeRegistry(t *testing.T, tokenBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "repository:library/grafana:pull", r.URL.Query().Get("scope"))
		w.Write([]byte(tokenBody))
	})
	mux.HandleFunc("HEAD /v2/library/grafana/manifests/{tag}", manifest(server))
	t.Cleanup(server.Close)
	return server
}

func manifest(server *httptest.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer anon" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry.test",scope="repository:library/grafana:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.PathValue("tag") != "11.2.0" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest)
	}
}

func Test_Resolve_FollowsTheBearerChallenge(t *testing.T) {
	r := WithBaseURL(fakeRegistry(t, `{"token":"anon"}`).URL)

	got, err := r.Resolve(context.Background(), "grafana", "11.2.0")
	_, missing := r.Resolve(context.Background(), "docker.io/grafana", "nope")

	require.NoError(t, err)
	require.Equal(t, digest, got)
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_Resolve_AcceptsEitherTokenField(t *testing.T) {
	bodies := []string{`{"token":"anon","access_token":"anon"}`, `{"access_token":"anon"}`}
	for _, body := range bodies {
		r := WithBaseURL(fakeRegistry(t, body).URL)

		got, err := r.Resolve(context.Background(), "grafana", "11.2.0")

		require.NoError(t, err, body)
		require.Equal(t, digest, got, body)
	}
}

func Test_Split(t *testing.T) {
	cases := map[string][2]string{
		"grafana":                       {"registry-1.docker.io", "library/grafana"},
		"grafana/grafana":               {"registry-1.docker.io", "grafana/grafana"},
		"docker.io/adrianozdp/plateful": {"registry-1.docker.io", "adrianozdp/plateful"},
		"ghcr.io/adrianozp/platform":    {"ghcr.io", "adrianozp/platform"},
		"localhost:5000/team/api":       {"localhost:5000", "team/api"},
	}
	for image, want := range cases {
		host, name := split(image)
		require.Equal(t, want, [2]string{host, name}, image)
	}
}

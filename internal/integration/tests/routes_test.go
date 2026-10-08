package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func runtimeForm(csrf string, route url.Values) url.Values {
	form := url.Values{
		"_csrf": {csrf}, "port": {"8080"}, "health_path": {"/healthz"}, "replicas": {"1"},
		"memory_mb": {"256"}, "cpu_millis": {"250"},
	}
	for key, values := range route {
		form[key] = values
	}
	return form
}

func Test_TheRuntimeFormSavesAliasesAndInternal(t *testing.T) {
	_, p := setupPlaces(t)
	base := "/services/places-api/env/dev"

	public := p.bob.Form(t, base+"/spec", runtimeForm(p.bob.CSRFToken, url.Values{"host": {"web.kalz.one"}, "aliases": {" WWW.kalz.one, "}}))
	publicPage := p.bob.Do(t, http.MethodGet, base, nil)
	internal := p.bob.Form(t, base+"/spec", runtimeForm(p.bob.CSRFToken, url.Values{"internal": {"on"}}))
	internalPage := p.bob.Do(t, http.MethodGet, base, nil)

	require.Equal(t, http.StatusNoContent, public.Status, public.Body)
	require.Contains(t, publicPage.Body, "web.kalz.one, www.kalz.one")
	require.Equal(t, http.StatusNoContent, internal.Status, internal.Body)
	require.Contains(t, internalPage.Body, "internal · this environment")
}

func Test_OnlyOneServicePerEnvironmentIsInternal(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "places-worker", "name": "Places worker", "owner_team": "places", "image_repository": "docker.io/adrianozdp/places-api",
	}, http.StatusCreated, nil)
	internal := map[string]any{"spec": map[string]any{
		"port": 8080, "health_path": "/healthz", "internal": true, "memory_mb": 256, "cpu_millis": 250, "replicas": 1,
	}}

	first := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", internal)
	second := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-worker/dev/revisions", internal)
	otherEnv := p.mia.Do(t, http.MethodPost, "/api/v1/config/places-worker/prod/revisions", internal)

	require.Equal(t, http.StatusCreated, first.Status, first.Body)
	require.Equal(t, http.StatusConflict, second.Status, second.Body)
	require.Equal(t, http.StatusCreated, otherEnv.Status, otherEnv.Body)
}

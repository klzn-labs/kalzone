package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ABuildIsSharedWithServicesOfTheSameImage(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "places-worker", "name": "Places worker", "owner_team": "places",
		"github_repo": repo, "image_repository": "docker.io/adrianozdp/places-api",
	}, http.StatusCreated, nil)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)

	built := callback(t, s, rel.ID, "built", repo, map[string]string{"digest": digest, "commit_sha": commit, "tag": "r1-0123456", "run_url": runURL})
	var listed struct {
		Releases []release `json:"releases"`
	}
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases?service=places-worker", nil, http.StatusOK, &listed)
	shared := listed.Releases
	page := p.bob.Do(t, http.MethodGet, "/services/places-worker/releases", nil)

	require.Equal(t, http.StatusOK, built.Status, built.Body)
	require.Len(t, shared, 1)
	require.Equal(t, "built", shared[0].Status)
	require.Equal(t, "docker.io/adrianozdp/places-api@"+digest, shared[0].Reference)
	require.Contains(t, page.Body, "shared from a build of the same image")
}

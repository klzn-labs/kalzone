package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type resourceDTO struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Environment string   `json:"environment"`
	Attached    []string `json:"attached"`
}

type resourcePage struct {
	Items []resourceDTO `json:"items"`
}

func Test_ResourcesListQueuesWithAttachedServices(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "money", "name": "Money"}, http.StatusCreated, nil)
	outsider := agentFor(t, s, p.root, map[string]any{"name": "money-reader", "scopes": []string{"read"}, "teams": []string{"money"}})
	var mine, theirs resourcePage

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/resources", nil, http.StatusOK, &mine)
	outsider.MustJSON(t, http.MethodGet, "/api/v1/resources", nil, http.StatusOK, &theirs)
	invalid := p.bob.Do(t, http.MethodGet, "/api/v1/resources?kind=nope", nil)

	require.Equal(t, []resourceDTO{{Kind: "queue", Name: "place-import", Environment: "dev", Attached: []string{"places-api"}}}, mine.Items)
	require.Empty(t, theirs.Items)
	require.Equal(t, http.StatusBadRequest, invalid.Status, invalid.Body)
}

func Test_ResourcesToolIsListedForReadTokens(t *testing.T) {
	s, p := setupPlaces(t)

	tools := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("reader", "read")).Token))

	require.Contains(t, tools, "listResources")
}

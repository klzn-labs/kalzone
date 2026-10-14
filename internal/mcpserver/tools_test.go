package mcpserver

import (
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/stretchr/testify/require"
)

type thingDTO struct {
	ID string `json:"id"`
}

type thingBody struct {
	Color string `json:"color"`
}

type thingParams struct {
	ID string `path:"id"`
}

type queueParams struct {
	Env  string `path:"env"`
	Name string `path:"name"`
}

var deployThing = apispec.Operation{ID: "deployThing", Method: http.MethodPost, Path: "/things/:id/deployments", Summary: "Deploy a thing",
	Scope: scope.Deploy, Gated: true, Waits: true, Params: thingParams{}, Status: http.StatusAccepted, Response: apispec.OrChangeRequest[thingDTO]{}}

func testCatalog() *apispec.Catalog {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "listThings", Method: http.MethodGet, Path: "/things", Summary: "List things", Scope: scope.Read, Status: http.StatusOK, Response: apispec.Page[thingDTO]{}})
	c.Add(apispec.Operation{ID: "putThing", Method: http.MethodPut, Path: "/things/:id", Summary: "Put a thing", Scope: scope.Services, Params: thingParams{}, Body: thingBody{}, Status: http.StatusOK, Response: thingDTO{}})
	c.Add(apispec.Operation{ID: "dropThing", Method: http.MethodDelete, Path: "/things/:id", Summary: "Drop a thing", Scope: scope.Services, Params: thingParams{}, Status: http.StatusNoContent})
	c.Add(apispec.Operation{ID: "purgeDeadLetters", Method: http.MethodDelete, Path: "/queues/:env/:name/dead", Summary: "Purge", Scope: scope.Services, Params: queueParams{}, Status: http.StatusNoContent})
	c.Add(apispec.Operation{ID: "getWhoami", Method: http.MethodGet, Path: "/whoami", Summary: "Who", Status: http.StatusOK, Response: thingDTO{}})
	c.Add(apispec.Operation{ID: "getMe", Method: http.MethodGet, Path: "/me", Summary: "Me", Session: true, Status: http.StatusOK, Response: thingDTO{}})
	c.Add(deployThing)
	return c
}

func testTools(t *testing.T) map[string]Tool {
	t.Helper()
	c := testCatalog()
	raw, err := apispec.Build(c, "https://platform.test")
	require.NoError(t, err)
	tools, err := Build(c, raw)
	require.NoError(t, err)
	byName := map[string]Tool{}
	for _, tool := range tools {
		byName[tool.Definition.Name] = tool
	}
	return byName
}

func Test_Build_MakesATool_PerOperationExceptTheExcludedOnes(t *testing.T) {
	tools := testTools(t)

	require.Len(t, tools, 6)
	require.NotContains(t, tools, "getMe")
	require.True(t, Excluded("getMe"))
	require.False(t, Excluded("listThings"))
}

func Test_Build_DescribesTheOperationWithItsNotesAndScope(t *testing.T) {
	tools := testTools(t)

	require.Equal(t, "Deploy a thing. "+apispec.Notes(deployThing)+" Needs scope deploy.", tools["deployThing"].Definition.Description)
	require.Equal(t, "List things. Needs scope read.", tools["listThings"].Definition.Description)
	require.Equal(t, "Who.", tools["getWhoami"].Definition.Description)
}

func Test_Build_AnnotatesByMethod(t *testing.T) {
	tools := testTools(t)

	require.True(t, tools["listThings"].Definition.Annotations.ReadOnlyHint)
	require.True(t, tools["putThing"].Definition.Annotations.IdempotentHint)
	require.True(t, *tools["dropThing"].Definition.Annotations.DestructiveHint)
	require.Nil(t, tools["putThing"].Definition.Annotations.DestructiveHint)
	require.False(t, *tools["putThing"].Definition.Annotations.OpenWorldHint)
}

func Test_Build_AsksPurgeForTheQueueNameAgain(t *testing.T) {
	purge := testTools(t)["purgeDeadLetters"]
	schema := purge.Definition.InputSchema.(map[string]any)

	require.Contains(t, schema["properties"], "confirm")
	require.Contains(t, schema["required"], "confirm")
}

func Test_VisibleTo_FollowsTheTokenScopes(t *testing.T) {
	tools := testTools(t)
	reader := actor.Actor{UserID: "u1", Token: actor.Token{ID: "k1", Scopes: []scope.Scope{scope.Read}}}

	require.True(t, tools["listThings"].VisibleTo(reader))
	require.False(t, tools["putThing"].VisibleTo(reader))
	require.True(t, tools["getWhoami"].VisibleTo(reader))
}

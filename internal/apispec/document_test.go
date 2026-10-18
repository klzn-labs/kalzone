package apispec_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

type thingDTO struct {
	ID     string            `json:"id"`
	Status string            `json:"status" enum:"new,done"`
	Note   string            `json:"note,omitempty"`
	Tags   []string          `json:"tags"`
	Labels map[string]string `json:"labels"`
	Extra  *extraDTO         `json:"extra,omitempty"`
}

type extraDTO struct {
	Value string `json:"value"`
}

type thingBody struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color,omitempty"`
}

type thingParams struct {
	ID string `path:"id"`
}

type listParams struct {
	Limit  int    `query:"limit"`
	Cursor string `query:"cursor"`
}

func catalog() *apispec.Catalog {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "listThings", Method: http.MethodGet, Path: "/things", Summary: "List things", Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[thingDTO]{}})
	c.Add(apispec.Operation{ID: "updateThing", Method: http.MethodPatch, Path: "/things/:id", Summary: "Change a thing", Scope: scope.Config, Gated: true, Params: thingParams{}, Body: thingBody{}, Status: http.StatusOK, Response: thingDTO{}})
	c.Add(apispec.Operation{ID: "deleteThing", Method: http.MethodDelete, Path: "/things/:id", Summary: "Delete a thing", Session: true, Params: thingParams{}, Status: http.StatusNoContent})
	return c
}

func build(t *testing.T, c *apispec.Catalog) map[string]any {
	t.Helper()
	raw, err := apispec.Build(c, "https://platform.test")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

func at(doc map[string]any, keys ...string) any {
	var current any = doc
	for _, key := range keys {
		current = current.(map[string]any)[key]
	}
	return current
}

func Test_Build_DescribesOperationsWithScopesAndErrors(t *testing.T) {
	doc := build(t, catalog())
	update := at(doc, "paths", "/api/v1/things/{id}", "patch")

	require.Equal(t, "3.1.0", doc["openapi"])
	require.Equal(t, "updateThing", at(update.(map[string]any), "operationId"))
	require.Equal(t, []any{map[string]any{"token": []any{"config"}}, map[string]any{"session": []any{}}}, at(update.(map[string]any), "security"))
	require.NotNil(t, at(update.(map[string]any), "responses", "202"))
	require.NotNil(t, at(update.(map[string]any), "responses", "default"))
	require.Equal(t, []any{map[string]any{"session": []any{}}}, at(doc, "paths", "/api/v1/things/{id}", "delete", "security"))
	require.Equal(t, "https://platform.test", at(doc, "servers").([]any)[0].(map[string]any)["url"])
}

func Test_Build_AppliesTheSchemaRules(t *testing.T) {
	schemas := at(build(t, catalog()), "components", "schemas").(map[string]any)
	thing := schemas["Apispec_testThing"].(map[string]any)
	body := schemas["Apispec_testThingBody"].(map[string]any)

	require.ElementsMatch(t, []any{"id", "status", "tags", "labels"}, thing["required"])
	require.Equal(t, false, thing["additionalProperties"])
	require.Equal(t, "array", at(thing, "properties", "tags", "type"))
	require.Equal(t, "object", at(thing, "properties", "labels", "type"))
	require.Equal(t, []any{"new", "done"}, at(thing, "properties", "status", "enum"))
	require.Equal(t, []any{"name"}, body["required"])
	require.Equal(t, false, schemas["Apispec_testExtra"].(map[string]any)["additionalProperties"])
	require.Contains(t, schemas, "Apispec_testThingPage")
}

func Test_Build_RefusesAnIncompleteOperation(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "noSummary", Method: http.MethodGet, Path: "/x", Status: http.StatusOK})

	_, err := apispec.Build(c, "https://platform.test")

	require.Error(t, err)
}

func Test_Build_RefusesAPathParameterWithoutItsDeclaration(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "getThing", Method: http.MethodGet, Path: "/things/:id", Summary: "Show a thing", Status: http.StatusOK, Response: thingDTO{}})

	_, err := apispec.Build(c, "https://platform.test")

	require.ErrorContains(t, err, "undefined path parameter: id")
}

func Test_DocPath_UsesOpenAPIParameters(t *testing.T) {
	require.Equal(t, "/api/v1/queues/{env}/{name}/dead/{id}/replay", apispec.DocPath("/queues/:env/:name/dead/:id/replay"))
}

func Test_Build_NeedsAGatedAcceptedAnswerToIncludeTheChangeRequest(t *testing.T) {
	plain := apispec.NewCatalog()
	plain.Add(apispec.Operation{ID: "startThing", Method: http.MethodPost, Path: "/things", Summary: "Start a thing", Gated: true, Status: http.StatusAccepted, Response: thingDTO{}})
	either := apispec.NewCatalog()
	either.Add(apispec.Operation{ID: "startThing", Method: http.MethodPost, Path: "/things", Summary: "Start a thing", Gated: true, Status: http.StatusAccepted, Response: apispec.OrChangeRequest[thingDTO]{}})

	_, refused := apispec.Build(plain, "https://platform.test")
	schemas := at(build(t, either), "components", "schemas").(map[string]any)

	require.Error(t, refused)
	require.Len(t, at(schemas, "Apispec_testThingOrChangeRequest", "oneOf"), 2)
}

func Test_Build_DocumentsWaitOnBothSidesOfTheAnswer(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "getThing", Method: http.MethodGet, Path: "/things/:id", Summary: "Show a thing", Waits: true, Params: thingParams{}, Status: http.StatusOK, Response: thingDTO{}})
	c.Add(apispec.Operation{ID: "startThing", Method: http.MethodPost, Path: "/things", Summary: "Start a thing", Gated: true, Waits: true, Status: http.StatusAccepted, Response: apispec.OrChangeRequest[thingDTO]{}})
	doc := build(t, c)
	get := at(doc, "paths", "/api/v1/things/{id}", "get").(map[string]any)
	start := at(doc, "paths", "/api/v1/things", "post").(map[string]any)

	require.NotNil(t, at(get, "responses", "202"))
	require.NotNil(t, at(start, "responses", "200"))
	require.Equal(t, "wait", at(start, "parameters").([]any)[0].(map[string]any)["name"])
	require.Contains(t, get["description"], "Retry-After")
	require.Contains(t, start["description"], "change request")
}

func Test_Build_DescribesABatchAnswerAsOneOfThree(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "startThings", Method: http.MethodPost, Path: "/things", Summary: "Start things", Gated: true, Status: http.StatusAccepted, Response: apispec.BatchOrChangeRequest[thingDTO]{}})

	schemas := at(build(t, c), "components", "schemas").(map[string]any)

	require.Len(t, at(schemas, "Apispec_testThingBatchOrChangeRequest", "oneOf"), 3)
	require.Contains(t, schemas, "Apispec_testThingPage")
}

func Test_Build_DeclaresRetryAfterOnEveryWaited202(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "getThing", Method: http.MethodGet, Path: "/things/:id", Summary: "Show a thing", Waits: true, Params: thingParams{}, Status: http.StatusOK, Response: thingDTO{}})
	c.Add(apispec.Operation{ID: "startThing", Method: http.MethodPost, Path: "/things", Summary: "Start a thing", Gated: true, Waits: true, Status: http.StatusAccepted, Response: apispec.OrChangeRequest[thingDTO]{}})
	doc := build(t, c)

	require.NotNil(t, at(doc, "paths", "/api/v1/things/{id}", "get", "responses", "202", "headers", "Retry-After"))
	require.NotNil(t, at(doc, "paths", "/api/v1/things", "post", "responses", "202", "headers", "Retry-After"))
	require.Nil(t, at(doc, "paths", "/api/v1/things", "post", "responses", "200", "headers"))
}

func Test_Build_KeepsTheChangeRequestOfAWaitedGatedCall(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "updateThing", Method: http.MethodPatch, Path: "/things/:id", Summary: "Change a thing", Gated: true, Waits: true, Params: thingParams{}, Status: http.StatusOK, Response: thingDTO{}})
	update := at(build(t, c), "paths", "/api/v1/things/{id}", "patch").(map[string]any)

	require.Contains(t, at(update, "responses", "202", "content", "application/json", "schema", "$ref"), "Pending")
	require.NotNil(t, at(update, "responses", "202", "headers", "Retry-After"))
	require.Contains(t, update["description"], "follows the change request")
}

func Test_Build_DocumentsTheBodysContentType(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "patchThing", Method: http.MethodPatch, Path: "/things/:id", Summary: "Patch a thing", Params: thingParams{},
		Body: thingBody{}, BodyType: "application/merge-patch+json", Status: http.StatusOK, Response: thingDTO{}})

	content := at(build(t, c), "paths", "/api/v1/things/{id}", "patch", "requestBody", "content").(map[string]any)

	require.Contains(t, content, "application/merge-patch+json")
	require.NotContains(t, content, "application/json")
}

func Test_Build_DescribesAMergePatchBodyAsTheObjectItDecodesInto(t *testing.T) {
	c := apispec.NewCatalog()
	c.Add(apispec.Operation{ID: "patchThing", Method: http.MethodPatch, Path: "/things/:id", Summary: "Patch a thing", Params: thingParams{},
		Body: thingBody{}, BodyType: "application/merge-patch+json", Status: http.StatusOK, Response: thingDTO{}})

	doc := build(t, c)
	schema := at(doc, "paths", "/api/v1/things/{id}", "patch", "requestBody", "content", "application/merge-patch+json", "schema")

	require.Equal(t, map[string]any{"$ref": "#/components/schemas/Apispec_testThingBody"}, schema)
	require.Contains(t, at(doc, "components", "schemas"), "Apispec_testThingBody")
}

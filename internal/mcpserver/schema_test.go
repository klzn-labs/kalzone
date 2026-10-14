package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const fixture = `{
  "paths": {
    "/api/v1/queues/{env}/{name}": {
      "post": {
        "operationId": "changeQueue",
        "parameters": [
          {"name": "env", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "wait", "in": "query", "schema": {"type": "string"}},
          {"name": "limit", "in": "query", "schema": {"type": "integer"}}
        ],
        "requestBody": {"content": {"application/merge-patch+json": {"schema": {"$ref": "#/components/schemas/Change"}}}}
      }
    },
    "/api/v1/raw": {
      "put": {
        "operationId": "putRaw",
        "requestBody": {"content": {"application/json": {"schema": {"type": "string"}}}}
      }
    },
    "/api/v1/clash/{name}": {
      "put": {
        "operationId": "clash",
        "parameters": [
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "query", "schema": {"type": "string"}}
        ]
      }
    },
    "/api/v1/nest/{name}": {
      "put": {
        "operationId": "nest",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Named"}}}}
      }
    }
  },
  "components": {
    "schemas": {
      "Change": {"type": "object", "required": ["targets"], "properties": {
        "targets": {"type": "array", "items": {"$ref": "#/components/schemas/Target"}},
        "message": {"type": "string"}
      }},
      "Target": {"type": "object", "properties": {"thing": {"$ref": "#/components/schemas/Thing"}}},
      "Thing": {"type": "object", "properties": {"id": {"type": "string"}}},
      "Named": {"type": "object", "properties": {"name": {"type": "string"}}}
    }
  }
}`

func fixtureInput(t *testing.T, path, method string) (input, error) {
	t.Helper()
	doc, err := parseDocument([]byte(fixture))
	require.NoError(t, err)
	op, ok := doc.operation(path, method)
	require.True(t, ok)
	return inputOf(doc, op)
}

func properties(in input) map[string]any {
	return in.Schema["properties"].(map[string]any)
}

func Test_InputOf_MergesParametersAndBodyFields(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/queues/{env}/{name}", "post")

	require.NoError(t, err)
	require.Equal(t, []string{"env", "name"}, in.Path)
	require.Equal(t, []string{"wait", "limit"}, in.Query)
	require.Equal(t, []string{"message", "targets"}, in.Fields)
	require.Equal(t, bodyFields, in.Body)
	require.Equal(t, "application/merge-patch+json", in.ContentType)
	require.ElementsMatch(t, []string{"env", "name", "targets"}, in.Schema["required"])
	require.Equal(t, "object", in.Schema["type"])
	require.Equal(t, false, in.Schema["additionalProperties"])
	require.Equal(t, map[string]any{"type": "integer"}, properties(in)["limit"])
}

func Test_InputOf_TurnsWaitIntoABoolean(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/queues/{env}/{name}", "post")

	require.NoError(t, err)
	require.Equal(t, "boolean", properties(in)["wait"].(map[string]any)["type"])
}

func Test_InputOf_CollectsNestedDefinitionsAndRewritesTheirReferences(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/queues/{env}/{name}", "post")

	require.NoError(t, err)
	defs := in.Schema["$defs"].(map[string]any)
	require.Contains(t, defs, "Target")
	require.Contains(t, defs, "Thing")
	require.NotContains(t, defs, "Named")
	require.Equal(t, "#/$defs/Target", properties(in)["targets"].(map[string]any)["items"].(map[string]any)["$ref"])
	require.Equal(t, "#/$defs/Thing", defs["Target"].(map[string]any)["properties"].(map[string]any)["thing"].(map[string]any)["$ref"])
}

func Test_InputOf_LeavesTheDocumentAsItWas(t *testing.T) {
	doc, err := parseDocument([]byte(fixture))
	require.NoError(t, err)
	op, _ := doc.operation("/api/v1/queues/{env}/{name}", "post")

	_, err = inputOf(doc, op)

	require.NoError(t, err)
	target := doc.Components.Schemas["Target"].(map[string]any)
	require.Equal(t, "#/components/schemas/Thing", target["properties"].(map[string]any)["thing"].(map[string]any)["$ref"])
}

func Test_InputOf_SendsABodyThatIsNotAnObjectAsBody(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/raw", "put")

	require.NoError(t, err)
	require.Equal(t, bodyWhole, in.Body)
	require.Equal(t, map[string]any{"type": "string"}, properties(in)["body"])
	require.Equal(t, []string{"body"}, in.Schema["required"])
}

func Test_InputOf_NestsTheBodyWhenAFieldHasAParametersName(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/nest/{name}", "put")

	require.NoError(t, err)
	require.Equal(t, bodyWhole, in.Body)
	require.Equal(t, map[string]any{"$ref": "#/$defs/Named"}, properties(in)["body"])
	require.Contains(t, in.Schema["$defs"], "Named")
}

func Test_InputOf_RefusesAParameterNameUsedTwice(t *testing.T) {
	_, err := fixtureInput(t, "/api/v1/clash/{name}", "put")

	require.ErrorIs(t, err, errCollision)
}

func Test_InputOf_MarshalsToJSON(t *testing.T) {
	in, err := fixtureInput(t, "/api/v1/queues/{env}/{name}", "post")
	require.NoError(t, err)

	_, err = json.Marshal(in.Schema)

	require.NoError(t, err)
}

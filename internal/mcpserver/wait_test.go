package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func waitFor(t *testing.T, raw string) *mcp.CallToolResult {
	t.Helper()
	args, err := arguments(json.RawMessage(raw))
	require.NoError(t, err)
	return waitTool().run(context.Background(), newEchoEngine(), origin(), args)
}

func uriOf(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	require.False(t, r.IsError, textOf(r, 0))
	var answer struct {
		URI string `json:"uri"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(r, 0)), &answer))
	return answer.URI
}

func Test_Wait_ReadsTheThingWithWait(t *testing.T) {
	require.Equal(t, "/api/v1/deployments/d1?wait=30s", uriOf(t, waitFor(t, `{"kind":"deployment","id":"d1","seconds":30}`)))
}

func Test_Wait_WaitsTheLongestByDefault(t *testing.T) {
	require.Equal(t, "/api/v1/deployments/d1?wait=120s", uriOf(t, waitFor(t, `{"kind":"deployment","id":"d1"}`)))
}

func Test_Wait_RefusesWrongArguments(t *testing.T) {
	for name, raw := range map[string]string{
		"kind":    `{"kind":"queue","id":"q1"}`,
		"long":    `{"kind":"deployment","id":"d1","seconds":500}`,
		"short":   `{"kind":"deployment","id":"d1","seconds":0}`,
		"no id":   `{"kind":"deployment"}`,
		"unknown": `{"kind":"deployment","id":"d1","for":"ever"}`,
	} {
		require.True(t, waitFor(t, raw).IsError, name)
	}
}

func Test_WaitTool_NeedsRead(t *testing.T) {
	tool := waitTool()

	require.Equal(t, "wait", tool.Definition.Name)
	require.Equal(t, "read", string(tool.Scope))
	require.True(t, tool.Definition.Annotations.ReadOnlyHint)
}

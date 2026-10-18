package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	waitName       = "wait"
	longestSeconds = 120
)

var waitPaths = map[string]string{"release": "/releases/:id", "deployment": "/deployments/:id", "request": "/requests/:id"}

var waitArguments = []string{"kind", "id", "seconds"}

var waitInputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"kind":    map[string]any{"type": "string", "enum": []any{"release", "deployment", "request"}, "description": "what the id names"},
		"id":      map[string]any{"type": "string", "description": "the release, deployment or change request id"},
		"seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": longestSeconds, "default": longestSeconds, "description": "how long to wait at most"},
	},
	"required":             []string{"kind", "id"},
	"additionalProperties": false,
}

func waitTool() Tool {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(bool)}
	definition := &mcp.Tool{
		Name:        waitName,
		Description: "Wait until a release is built, a deployment finishes or a change request is decided, then answer with it. Needs scope read.",
		InputSchema: waitInputSchema, Annotations: readOnly,
	}
	return Tool{Definition: definition, Scope: scope.Read, run: runWait}
}

func runWait(ctx context.Context, engine http.Handler, origin *http.Request, args map[string]any) *mcp.CallToolResult {
	if err := checkNames(args, func(name string) bool { return slices.Contains(waitArguments, name) }); err != nil {
		return failure(err)
	}
	path, ok := waitPaths[stringOf(args["kind"])]
	if !ok {
		return failure(apperror.Invalid("kind must be release, deployment or request").WithField("kind"))
	}
	id := stringOf(args["id"])
	if id == "" {
		return failure(apperror.Invalid("id is required").WithField("id"))
	}
	seconds, err := secondsOf(args["seconds"])
	if err != nil {
		return failure(err)
	}
	get := call{method: http.MethodGet, path: path, input: input{Path: []string{"id"}, Query: []string{waitProperty}}}
	return get.run(ctx, engine, origin, map[string]any{"id": id, waitProperty: strconv.Itoa(seconds) + "s"})
}

func stringOf(value any) string {
	s, _ := value.(string)
	return s
}

func secondsOf(value any) (int, *apperror.AppError) {
	if value == nil {
		return longestSeconds, nil
	}
	number, ok := value.(json.Number)
	seconds, err := number.Int64()
	if !ok || err != nil || seconds < 1 || seconds > longestSeconds {
		return 0, apperror.Invalid("seconds must be a whole number from 1 to 120").WithField("seconds")
	}
	return int(seconds), nil
}

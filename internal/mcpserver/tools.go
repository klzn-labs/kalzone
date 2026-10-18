package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	purgeID         = "purgeDeadLetters"
	confirmProperty = "confirm"
	confirmedName   = "name"
)

var errUndocumented = errors.New("the operation is missing from the document")

var excluded = map[string]bool{
	"getOpenAPI": true, "getMe": true, "listTokens": true, "createToken": true, "revokeToken": true,
	"createTeam": true, "deleteTeam": true, "putMember": true, "removeMember": true,
	"listUsers": true, "getUser": true, "setAdmin": true,
	"deleteService": true, "transferService": true, "deleteQueue": true, "unsetSecret": true,
}

type call struct {
	method  string
	path    string
	input   input
	confirm string
}

type run func(ctx context.Context, engine http.Handler, origin *http.Request, args map[string]any) *mcp.CallToolResult

type Tool struct {
	Definition *mcp.Tool
	Scope      scope.Scope
	run        run
}

func Excluded(id string) bool {
	return excluded[id]
}

func Build(catalog *apispec.Catalog, raw []byte) ([]Tool, error) {
	doc, err := parseDocument(raw)
	if err != nil {
		return nil, err
	}
	tools := []Tool{}
	for _, op := range catalog.Operations() {
		if excluded[op.ID] {
			continue
		}
		tool, err := toolFor(doc, op)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", op.ID, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (t Tool) VisibleTo(a actor.Actor) bool {
	return t.Scope == "" || a.Token.Has(t.Scope)
}

func toolFor(doc document, op apispec.Operation) (Tool, error) {
	opDoc, ok := doc.operation(apispec.DocPath(op.Path), strings.ToLower(op.Method))
	if !ok {
		return Tool{}, errUndocumented
	}
	in, err := inputOf(doc, opDoc)
	if err != nil {
		return Tool{}, err
	}
	c := call{method: op.Method, path: op.Path, input: in}
	if op.ID == purgeID {
		c = withConfirm(c)
	}
	return Tool{Definition: definition(op, c.input), Scope: op.Scope, run: c.run}, nil
}

func withConfirm(c call) call {
	props := c.input.Schema["properties"].(map[string]any)
	props[confirmProperty] = map[string]any{"type": "string", "description": "the queue's name again, to confirm the purge"}
	required, _ := c.input.Schema["required"].([]string)
	c.input.Schema["required"] = append(required, confirmProperty)
	c.confirm = confirmedName
	return c
}

func definition(op apispec.Operation, in input) *mcp.Tool {
	return &mcp.Tool{Name: op.ID, Description: description(op), InputSchema: in.Schema, Annotations: annotations(op.Method)}
}

func description(op apispec.Operation) string {
	parts := []string{op.Summary + "."}
	if notes := apispec.Notes(op); notes != "" {
		parts = append(parts, notes)
	}
	if op.Scope != "" {
		parts = append(parts, "Needs scope "+string(op.Scope)+".")
	}
	return strings.Join(parts, " ")
}

func annotations(method string) *mcp.ToolAnnotations {
	closed := false
	a := &mcp.ToolAnnotations{OpenWorldHint: &closed}
	switch method {
	case http.MethodGet:
		a.ReadOnlyHint = true
	case http.MethodPut:
		a.IdempotentHint = true
	case http.MethodDelete:
		destructive := true
		a.DestructiveHint = &destructive
	}
	return a
}

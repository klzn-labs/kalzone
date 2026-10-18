package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	notFinal = "Not final yet: call the wait tool with the id in this answer."
	done     = "Done."
)

func result(recorder *httptest.ResponseRecorder) *mcp.CallToolResult {
	if recorder.Code == http.StatusNoContent {
		return text(done)
	}
	r := text(recorder.Body.String())
	r.IsError = recorder.Code >= http.StatusBadRequest
	if recorder.Code == http.StatusAccepted && recorder.Header().Get("Retry-After") != "" {
		r.Content = append(r.Content, &mcp.TextContent{Text: notFinal})
	}
	return r
}

func failure(err *apperror.AppError) *mcp.CallToolResult {
	raw, _ := json.Marshal(httperror.Body{Error: err})
	r := text(string(raw))
	r.IsError = true
	return r
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

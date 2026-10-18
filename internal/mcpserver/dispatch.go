package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const waitLongest = "120s"

var (
	ginParam  = regexp.MustCompile(`:([a-zA-Z_]+)`)
	forwarded = []string{"Authorization", "X-Forwarded-For", "X-Real-IP", "User-Agent"}
)

func (t Tool) handler(engine http.Handler, origin *http.Request) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := arguments(req.Params.Arguments)
		if err != nil {
			return failure(apperror.Invalid("the arguments must be a JSON object").WithHint(err.Error())), nil
		}
		return t.run(ctx, engine, origin, args), nil
	}
}

func arguments(raw json.RawMessage) (map[string]any, error) {
	args := map[string]any{}
	if len(raw) == 0 || string(raw) == "null" {
		return args, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return args, decoder.Decode(&args)
}

func (c call) run(ctx context.Context, engine http.Handler, origin *http.Request, args map[string]any) *mcp.CallToolResult {
	if err := c.check(args); err != nil {
		return failure(err)
	}
	req, appErr := c.request(ctx, origin, args)
	if appErr != nil {
		return failure(appErr)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return result(recorder)
}

func (c call) check(args map[string]any) *apperror.AppError {
	if err := checkNames(args, c.accepts); err != nil {
		return err
	}
	return c.checkConfirm(args)
}

func checkNames(args map[string]any, accepts func(string) bool) *apperror.AppError {
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !accepts(name) {
			return apperror.Invalid("unknown argument " + name).WithField(name).WithHint("use only the properties of the tool's input schema")
		}
	}
	return nil
}

func (c call) accepts(name string) bool {
	in := c.input
	return slices.Contains(in.Path, name) || slices.Contains(in.Query, name) || slices.Contains(in.Fields, name) ||
		(in.Body == bodyWhole && name == bodyProperty) || (c.confirm != "" && name == confirmProperty)
}

func (c call) checkConfirm(args map[string]any) *apperror.AppError {
	if c.confirm == "" {
		return nil
	}
	if args[c.confirm] == nil || args[confirmProperty] != args[c.confirm] {
		return apperror.Invalid("confirm must be the queue's name").WithField(confirmProperty).WithHint("send confirm with the same value as " + c.confirm)
	}
	return nil
}

func (c call) request(ctx context.Context, origin *http.Request, args map[string]any) (*http.Request, *apperror.AppError) {
	path, appErr := fillPath(c.path, args)
	if appErr != nil {
		return nil, appErr
	}
	body, err := c.body(args)
	if err != nil {
		return nil, apperror.Invalid("the arguments do not make a JSON body").WithHint(err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, c.method, apispec.BasePath+path+c.query(args), body)
	if err != nil {
		return nil, apperror.Invalid("the arguments do not make a request").WithHint(err.Error())
	}
	c.headers(req, origin)
	return req, nil
}

func fillPath(path string, args map[string]any) (string, *apperror.AppError) {
	var problem *apperror.AppError
	filled := ginParam.ReplaceAllStringFunc(path, func(param string) string {
		value, err := pathValue(args, param[1:])
		if problem == nil {
			problem = err
		}
		return url.PathEscape(value)
	})
	if problem != nil {
		return "", problem
	}
	return filled, nil
}

func pathValue(args map[string]any, name string) (string, *apperror.AppError) {
	raw, ok := args[name]
	if !ok {
		return "", apperror.Invalid("missing argument " + name).WithField(name).WithHint("the tool's input schema lists it as required")
	}
	value := fmt.Sprint(raw)
	if strings.Contains(value, "/") {
		return "", apperror.Invalid(name + " cannot contain /").WithField(name).WithHint("send the slug or id itself")
	}
	return value, nil
}

func (c call) query(args map[string]any) string {
	q := url.Values{}
	for _, name := range c.input.Query {
		if value, ok := args[name]; ok {
			setQuery(q, name, value)
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func setQuery(q url.Values, name string, value any) {
	if name != waitProperty {
		q.Set(name, fmt.Sprint(value))
		return
	}
	switch wait := value.(type) {
	case bool:
		if wait {
			q.Set(waitProperty, waitLongest)
		}
	case string:
		q.Set(waitProperty, wait)
	}
}

func (c call) body(args map[string]any) (io.Reader, error) {
	switch c.input.Body {
	case bodyFields:
		return encode(pick(args, c.input.Fields))
	case bodyWhole:
		return encode(args[bodyProperty])
	}
	return nil, nil
}

func pick(args map[string]any, names []string) map[string]any {
	picked := map[string]any{}
	for _, name := range names {
		if value, ok := args[name]; ok {
			picked[name] = value
		}
	}
	return picked
}

func encode(value any) (io.Reader, error) {
	raw, err := json.Marshal(value)
	return bytes.NewReader(raw), err
}

func (c call) headers(req *http.Request, origin *http.Request) {
	for _, name := range forwarded {
		if value := origin.Header.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	req.RemoteAddr = origin.RemoteAddr
	req.Header.Set("Accept", "application/json")
	if c.input.Body != bodyNone {
		req.Header.Set("Content-Type", c.input.ContentType)
	}
}

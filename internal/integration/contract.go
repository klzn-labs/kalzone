package integration

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/adrianozp/platform/internal/apispec"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/require"
)

var buildCallback = regexp.MustCompile("^" + apispec.BasePath + "/releases/[^/]+/(started|built|failed|registry-credentials)$")

type contract struct {
	router routers.Router
}

func loadContract(t *testing.T, catalog *apispec.Catalog) *contract {
	t.Helper()
	raw, err := apispec.Build(catalog, "http://platform.test")
	require.NoError(t, err)
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	require.NoError(t, err)
	return &contract{router: router}
}

func (c *contract) check(t *testing.T, req *http.Request, resp Response) {
	t.Helper()
	if !documented(req.URL.Path) {
		return
	}
	route, params, err := c.router.FindRoute(req)
	require.NoError(t, err, "%s %s is not in the document", req.Method, req.URL.Path)
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 resp.Status, Header: resp.Header,
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	input.SetBodyBytes([]byte(resp.Body))
	err = openapi3filter.ValidateResponse(context.Background(), input)
	require.NoError(t, err, "%s %s answered %d %s", req.Method, req.URL.Path, resp.Status, resp.Body)
}

func documented(path string) bool {
	return IsAPI(path) && !buildCallback.MatchString(path)
}

func IsAPI(path string) bool {
	return path == apispec.BasePath || strings.HasPrefix(path, apispec.BasePath+"/")
}

package integration

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/stretchr/testify/require"
)

var buildCallback = regexp.MustCompile("^" + apispec.BasePath + "/releases/[^/]+/(started|built|failed|registry-credentials)$")

type contract struct {
	router routers.Router
}

var built struct {
	once     sync.Once
	contract *contract
	err      error
}

func loadContract(t *testing.T, catalog *apispec.Catalog) *contract {
	t.Helper()
	built.once.Do(func() { built.contract, built.err = buildContract(catalog) })
	require.NoError(t, built.err)
	return built.contract
}

func buildContract(catalog *apispec.Catalog) (*contract, error) {
	raw, err := apispec.Build(catalog, "http://platform.test")
	if err != nil {
		return nil, err
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	return &contract{router: router}, err
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

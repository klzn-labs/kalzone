package apispec

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi31"
)

const gatedNote = "On a protected environment the token also needs the protected scope; without bypass the call answers 202 with a change request."

var ginParam = regexp.MustCompile(`:([a-zA-Z_]+)`)

func Build(c *Catalog, baseURL string) ([]byte, error) {
	r := openapi31.NewReflector()
	r.DefaultOptions = append(r.DefaultOptions, schemaRules()...)
	r.Spec.Info.WithTitle("Platform API").WithVersion("1").WithDescription("The internal developer platform's API for people and agents.")
	r.Spec.WithServers(openapi31.Server{URL: baseURL})
	r.Spec.SetHTTPBearerTokenSecurity("token", "plat_…", "an access token from the Tokens page or POST /api/v1/tokens")
	r.Spec.SetAPIKeySecurity("session", "platform_session", openapi.InCookie, "a browser session; writes also need the X-CSRF-Token header")
	for _, op := range c.Operations() {
		if err := add(r, op); err != nil {
			return nil, fmt.Errorf("operation %s %s: %w", op.Method, op.Path, err)
		}
	}
	return r.Spec.MarshalJSON()
}

func DocPath(ginPath string) string {
	return BasePath + ginParam.ReplaceAllString(ginPath, "{$1}")
}

var (
	errIncomplete  = errors.New("an operation needs an ID, a summary and a status")
	errGatedAnswer = errors.New("a gated operation that answers 202 must answer apispec.OrChangeRequest")
)

func check(op Operation) error {
	if op.ID == "" || op.Summary == "" || op.Status == 0 {
		return errIncomplete
	}
	if _, either := op.Response.(jsonschema.OneOfExposer); op.Gated && op.Status == http.StatusAccepted && !either {
		return errGatedAnswer
	}
	return nil
}

func add(r *openapi31.Reflector, op Operation) error {
	if err := check(op); err != nil {
		return err
	}
	oc, err := r.NewOperationContext(op.Method, DocPath(op.Path))
	if err != nil {
		return err
	}
	describe(oc, op)
	request(oc, op)
	respond(oc, op)
	return r.AddOperation(oc)
}

func describe(oc openapi.OperationContext, op Operation) {
	oc.SetID(op.ID)
	oc.SetSummary(op.Summary)
	oc.SetTags(strings.Split(strings.TrimPrefix(op.Path, "/"), "/")[0])
	if op.Gated {
		oc.SetDescription(gatedNote)
	}
	if !op.Session {
		oc.AddSecurity("token", scopesOf(op)...)
	}
	oc.AddSecurity("session")
}

func scopesOf(op Operation) []string {
	if op.Scope == "" {
		return nil
	}
	return []string{string(op.Scope)}
}

func request(oc openapi.OperationContext, op Operation) {
	if op.Params != nil {
		oc.AddReqStructure(op.Params)
	}
	if op.Body != nil {
		oc.AddReqStructure(op.Body)
	}
}

func respond(oc openapi.OperationContext, op Operation) {
	oc.AddRespStructure(op.Response, openapi.WithHTTPStatus(op.Status))
	if op.Gated && op.Status != http.StatusAccepted {
		oc.AddRespStructure(new(approval.Pending), openapi.WithHTTPStatus(http.StatusAccepted))
	}
	oc.AddRespStructure(new(httperror.Body), func(cu *openapi.ContentUnit) {
		cu.IsDefault, cu.Description = true, "An error; error.hint says how to fix the call"
	})
}

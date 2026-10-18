package apispec

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi31"
)

const (
	gatedNote     = "On a protected environment the token also needs the protected scope; without bypass the call answers 202 with a change request."
	waitNote      = "With ?wait the call answers once the result is final (200), or when the wait ends with the current state (202 and Retry-After)."
	gatedWaitNote = "A waited call that became a change request follows the change request and answers with it."
	jsonType      = "application/json"
)

type WaitParams struct {
	Wait string `query:"wait" description:"wait up to this long for a final status, as a duration from just above 0 to 120s, such as 30s or 2m"`
}

var ginParam = regexp.MustCompile(`:([a-zA-Z_]+)`)

func Build(c *Catalog, baseURL string) ([]byte, error) {
	r := openapi31.NewReflector()
	r.DefaultOptions = append(r.DefaultOptions, schemaRules()...)
	r.Spec.Info.WithTitle("Kalzone API").WithVersion("1").WithDescription("The API of Kalzone, the internal developer platform, for people and agents.")
	r.Spec.WithServers(openapi31.Server{URL: baseURL})
	r.Spec.SetHTTPBearerTokenSecurity("token", "plat_…", "an access token from the Tokens page or POST /api/v1/tokens")
	r.Spec.SetAPIKeySecurity("session", "platform_session", openapi.InCookie, "a browser session; writes also need the X-CSRF-Token header")
	for _, op := range c.Operations() {
		if err := add(r, op); err != nil {
			return nil, fmt.Errorf("operation %s %s: %w", op.Method, op.Path, err)
		}
	}
	if err := moveBodyTypes(r.Spec, c.Operations()); err != nil {
		return nil, err
	}
	return r.Spec.MarshalJSON()
}

func moveBodyTypes(spec *openapi31.Spec, ops []Operation) error {
	for _, op := range ops {
		if op.BodyType == "" {
			continue
		}
		if err := spec.SetupOperation(op.Method, DocPath(op.Path), moveBodyType(op.BodyType)); err != nil {
			return err
		}
	}
	return nil
}

func moveBodyType(bodyType string) func(*openapi31.Operation) error {
	return func(o *openapi31.Operation) error {
		content := o.RequestBody.RequestBody.Content
		content[bodyType] = content[jsonType]
		delete(content, jsonType)
		return nil
	}
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
	if note := Notes(op); note != "" {
		oc.SetDescription(note)
	}
	if !op.Session {
		oc.AddSecurity("token", scopesOf(op)...)
	}
	oc.AddSecurity("session")
}

func Notes(op Operation) string {
	all := []string{noteIf(op.Gated, gatedNote), noteIf(op.Waits, waitNote), noteIf(op.Gated && op.Waits, gatedWaitNote)}
	return strings.TrimSpace(strings.Join(all, " "))
}

func noteIf(applies bool, note string) string {
	if !applies {
		return ""
	}
	return note
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
	if op.Waits {
		oc.AddReqStructure(WaitParams{})
	}
}

func waitedStatus(status int) int {
	if status == http.StatusAccepted {
		return http.StatusOK
	}
	return http.StatusAccepted
}

func respond(oc openapi.OperationContext, op Operation) {
	answer(oc, op, op.Response, op.Status)
	if op.Waits && !answersChangeRequest(op) {
		answer(oc, op, op.Response, waitedStatus(op.Status))
	}
	if answersChangeRequest(op) {
		answer(oc, op, new(approval.Pending), http.StatusAccepted)
	}
	oc.AddRespStructure(new(httperror.Body), func(cu *openapi.ContentUnit) {
		cu.IsDefault, cu.Description = true, "An error; error.hint says how to fix the call"
	})
}

func answersChangeRequest(op Operation) bool {
	return op.Gated && op.Status != http.StatusAccepted
}

func answer(oc openapi.OperationContext, op Operation, structure any, status int) {
	oc.AddRespStructure(structure, openapi.WithHTTPStatus(status), func(cu *openapi.ContentUnit) {
		if op.Waits && status == http.StatusAccepted {
			cu.Customize = declareRetryAfter
		}
	})
}

func declareRetryAfter(cor openapi.ContentOrReference) {
	resp, ok := cor.(*openapi31.ResponseOrReference)
	if !ok || resp.Response == nil {
		return
	}
	header := openapi31.Header{Schema: map[string]any{"type": "integer"}}
	header.WithDescription("seconds to wait before reading again; sent when ?wait ended before a final status")
	resp.Response.WithHeadersItem("Retry-After", openapi31.HeaderOrReference{Header: &header})
}

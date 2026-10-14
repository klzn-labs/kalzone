package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type bodyMode int

const (
	bodyNone bodyMode = iota
	bodyFields
	bodyWhole
)

const (
	componentRef = "#/components/schemas/"
	defsRef      = "#/$defs/"
	bodyProperty = "body"
	waitProperty = "wait"
)

var errCollision = errors.New("a property name is used twice")

var waitSchema = map[string]any{"type": "boolean", "description": "true waits up to 120s for a final status"}

type input struct {
	Schema      map[string]any
	Path        []string
	Query       []string
	Fields      []string
	Body        bodyMode
	ContentType string
}

type parameter struct {
	Name     string         `json:"name"`
	In       string         `json:"in"`
	Required bool           `json:"required"`
	Schema   map[string]any `json:"schema"`
}

type mediaType struct {
	Schema map[string]any `json:"schema"`
}

type requestBody struct {
	Content map[string]mediaType `json:"content"`
}

type operationDoc struct {
	ID          string       `json:"operationId"`
	Parameters  []parameter  `json:"parameters"`
	RequestBody *requestBody `json:"requestBody"`
}

type document struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]any `json:"schemas"`
	} `json:"components"`
}

func parseDocument(raw []byte) (document, error) {
	var doc document
	err := json.Unmarshal(raw, &doc)
	return doc, err
}

func (d document) operation(path, method string) (operationDoc, bool) {
	raw, ok := d.Paths[path][method]
	if !ok {
		return operationDoc{}, false
	}
	var op operationDoc
	return op, json.Unmarshal(raw, &op) == nil
}

type builder struct {
	in       input
	props    map[string]any
	required []string
}

func inputOf(doc document, op operationDoc) (input, error) {
	b := &builder{props: map[string]any{}}
	if err := b.addParameters(op.Parameters); err != nil {
		return input{}, err
	}
	if err := b.addBody(doc, op.RequestBody); err != nil {
		return input{}, err
	}
	b.in.Schema = b.schema(doc)
	return b.in, nil
}

func (b *builder) add(name string, schema any, required bool) error {
	if _, taken := b.props[name]; taken {
		return fmt.Errorf("%w: %s", errCollision, name)
	}
	b.props[name] = schema
	if required {
		b.required = append(b.required, name)
	}
	return nil
}

func (b *builder) addParameters(params []parameter) error {
	for _, p := range params {
		if err := b.add(p.Name, parameterSchema(p), p.Required); err != nil {
			return err
		}
		b.track(p)
	}
	return nil
}

func parameterSchema(p parameter) any {
	if p.In == "query" && p.Name == waitProperty {
		return waitSchema
	}
	return p.Schema
}

func (b *builder) track(p parameter) {
	if p.In == "path" {
		b.in.Path = append(b.in.Path, p.Name)
		return
	}
	b.in.Query = append(b.in.Query, p.Name)
}

func (b *builder) addBody(doc document, body *requestBody) error {
	contentType, schema, ok := onlyContent(body)
	if !ok {
		return nil
	}
	b.in.ContentType = contentType
	object := resolve(doc, schema)
	props, isObject := object["properties"].(map[string]any)
	if !isObject || len(props) == 0 || b.takesAny(props) {
		b.in.Body = bodyWhole
		return b.add(bodyProperty, schema, true)
	}
	b.in.Body = bodyFields
	return b.addFields(props, requiredOf(object))
}

func (b *builder) takesAny(props map[string]any) bool {
	for name := range props {
		if _, taken := b.props[name]; taken {
			return true
		}
	}
	return false
}

func onlyContent(body *requestBody) (string, map[string]any, bool) {
	if body == nil {
		return "", nil, false
	}
	for contentType, media := range body.Content {
		return contentType, media.Schema, true
	}
	return "", nil, false
}

func resolve(doc document, schema map[string]any) map[string]any {
	ref, ok := schema["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, componentRef) {
		return schema
	}
	target, _ := doc.Components.Schemas[strings.TrimPrefix(ref, componentRef)].(map[string]any)
	return target
}

func requiredOf(object map[string]any) []string {
	names := []string{}
	list, _ := object["required"].([]any)
	for _, name := range list {
		names = append(names, fmt.Sprint(name))
	}
	return names
}

func (b *builder) addFields(props map[string]any, required []string) error {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := b.add(name, props[name], slices.Contains(required, name)); err != nil {
			return err
		}
	}
	b.in.Fields = names
	return nil
}

func (b *builder) schema(doc document) map[string]any {
	schema := map[string]any{"type": "object", "properties": b.props, "additionalProperties": false}
	if len(b.required) > 0 {
		schema["required"] = b.required
	}
	if defs := collectDefs(doc, b.props); len(defs) > 0 {
		schema["$defs"] = defs
	}
	return rewrite(schema).(map[string]any)
}

func collectDefs(doc document, root any) map[string]any {
	defs := map[string]any{}
	queue := refsIn(root)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, seen := defs[name]; seen {
			continue
		}
		defs[name] = doc.Components.Schemas[name]
		queue = append(queue, refsIn(defs[name])...)
	}
	return defs
}

func refsIn(v any) []string {
	switch value := v.(type) {
	case map[string]any:
		return refsInObject(value)
	case []any:
		return refsInList(value)
	}
	return nil
}

func refsInObject(object map[string]any) []string {
	var refs []string
	if ref, ok := object["$ref"].(string); ok && strings.HasPrefix(ref, componentRef) {
		refs = append(refs, strings.TrimPrefix(ref, componentRef))
	}
	for _, item := range object {
		refs = append(refs, refsIn(item)...)
	}
	return refs
}

func refsInList(list []any) []string {
	var refs []string
	for _, item := range list {
		refs = append(refs, refsIn(item)...)
	}
	return refs
}

func rewrite(v any) any {
	switch value := v.(type) {
	case map[string]any:
		return rewriteObject(value)
	case []any:
		return rewriteList(value)
	case []string:
		return slices.Clone(value)
	}
	return v
}

func rewriteObject(object map[string]any) map[string]any {
	out := make(map[string]any, len(object))
	for key, item := range object {
		out[key] = rewrite(item)
	}
	if ref, ok := object["$ref"].(string); ok {
		out["$ref"] = strings.Replace(ref, componentRef, defsRef, 1)
	}
	return out
}

func rewriteList(list []any) []any {
	out := make([]any, len(list))
	for i, item := range list {
		out[i] = rewrite(item)
	}
	return out
}

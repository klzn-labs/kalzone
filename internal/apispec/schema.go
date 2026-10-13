package apispec

import (
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/swaggest/jsonschema-go"
)

func schemaRules() []func(*jsonschema.ReflectContext) {
	return []func(*jsonschema.ReflectContext){
		jsonschema.InterceptSchema(closeObjects),
		jsonschema.InterceptProp(requireUnlessOmitted),
		jsonschema.InterceptNullability(neverNullCollections),
		jsonschema.InterceptDefName(defName),
	}
}

func closeObjects(params jsonschema.InterceptSchemaParams) (bool, error) {
	if params.Processed && structType(params.Value.Type()) && len(params.Schema.Properties) > 0 {
		params.Schema.AdditionalProperties = (&jsonschema.SchemaOrBool{}).WithTypeBoolean(false)
	}
	return false, nil
}

func structType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}

func requireUnlessOmitted(params jsonschema.InterceptPropParams) error {
	tag, tagged := params.Field.Tag.Lookup("json")
	if !params.Processed || !tagged || tag == "-" || strings.Contains(tag, ",omitempty") {
		return nil
	}
	if !slices.Contains(params.ParentSchema.Required, params.Name) {
		params.ParentSchema.Required = append(params.ParentSchema.Required, params.Name)
	}
	return nil
}

func neverNullCollections(params jsonschema.InterceptNullabilityParams) {
	kind := params.Type.Kind()
	if params.NullAdded && (kind == reflect.Slice || kind == reflect.Map) {
		params.Schema.RemoveType(jsonschema.Null)
	}
}

func defName(t reflect.Type, _ string) string {
	if strings.HasPrefix(t.Name(), "Page[") && t.Kind() == reflect.Struct {
		return defName(t.Field(0).Type.Elem(), "") + "Page"
	}
	if strings.HasPrefix(t.Name(), "OrChangeRequest[") {
		return defName(answerOf(t), "") + "OrChangeRequest"
	}
	return pascal(packageName(t.PkgPath())) + pascal(strings.TrimSuffix(t.Name(), "DTO"))
}

func answerOf(t reflect.Type) reflect.Type {
	answer := reflect.Zero(t).Interface().(jsonschema.OneOfExposer).JSONSchemaOneOf()[0]
	return reflect.TypeOf(answer)
}

func packageName(pkgPath string) string {
	if path.Base(pkgPath) == "entities" {
		return path.Base(path.Dir(pkgPath))
	}
	return path.Base(pkgPath)
}

func pascal(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

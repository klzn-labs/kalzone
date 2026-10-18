package entities

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var patchKeys = []string{"spec", "env", "message"}

const patchShape = `send {"spec": {...}, "env": {"NAME": "value" or null}, "message": "..."}`

var (
	errPatchNotObject = apperror.Invalid("a merge patch must be a JSON object").WithHint(patchShape)
	errSpecRemoved    = apperror.Invalid("spec cannot be removed").WithField("spec").
				WithHint("send only the spec fields to change; null removes one field, not the whole spec")
	errEnvRemoved = apperror.Invalid("env cannot be removed as a whole").WithField("env").
			WithHint("set each variable to remove to null")
	errSpecNotObject  = apperror.Invalid("spec must be an object").WithField("spec").WithHint(patchShape)
	errEnvNotObject   = apperror.Invalid("env must be an object").WithField("env").WithHint(patchShape)
	errMessageNotText = apperror.Invalid("message must be a string").WithField("message")
	errEnvValue       = apperror.Invalid("env values must be strings, or null to remove one").WithField("env")
)

type patched struct {
	Spec Spec              `json:"spec"`
	Env  map[string]string `json:"env"`
}

func MergePatch(target, patch any) any {
	fields, isObject := patch.(map[string]any)
	if !isObject {
		return patch
	}
	merged, isObject := target.(map[string]any)
	if !isObject {
		merged = map[string]any{}
	}
	for key, value := range fields {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = MergePatch(merged[key], value)
	}
	return merged
}

func PatchChangeSet(current Revision, raw []byte) (ChangeSet, error) {
	patch, err := patchObject(raw)
	if err != nil {
		return ChangeSet{}, err
	}
	message, err := takeMessage(patch)
	if err != nil {
		return ChangeSet{}, err
	}
	result, err := applyPatch(current, patch)
	if err != nil {
		return ChangeSet{}, err
	}
	cs := ChangeSet{Message: message}
	if _, sent := patch["spec"]; sent {
		cs.Spec = &result.Spec
	}
	cs.SetEnv, cs.UnsetEnv = envChanges(current.Env, result.Env)
	return cs, nil
}

func patchObject(raw []byte) (map[string]any, error) {
	var patch map[string]any
	if err := json.Unmarshal(raw, &patch); err != nil || patch == nil {
		return nil, errPatchNotObject
	}
	if err := knownKeys(patch, "", patchKeys); err != nil {
		return nil, err
	}
	if err := checkSpec(patch); err != nil {
		return nil, err
	}
	return patch, checkEnv(patch)
}

func checkSpec(patch map[string]any) error {
	value, sent := patch["spec"]
	if !sent {
		return nil
	}
	if value == nil {
		return errSpecRemoved
	}
	fields, isObject := value.(map[string]any)
	if !isObject {
		return errSpecNotObject
	}
	return knownKeys(fields, "spec", specFields())
}

func checkEnv(patch map[string]any) error {
	value, sent := patch["env"]
	if !sent {
		return nil
	}
	if value == nil {
		return errEnvRemoved
	}
	if _, isObject := value.(map[string]any); !isObject {
		return errEnvNotObject
	}
	return nil
}

func knownKeys(fields map[string]any, scope string, accepted []string) error {
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(accepted, key) {
			return unknownField(scope, key, accepted)
		}
	}
	return nil
}

func unknownField(scope, key string, accepted []string) error {
	field, label := key, "a merge patch"
	if scope != "" {
		field, label = scope+"."+key, scope
	}
	return apperror.Invalid(fmt.Sprintf("unknown field %q", field)).WithField(field).
		WithHint(label + " accepts: " + strings.Join(accepted, ", "))
}

func takeMessage(patch map[string]any) (string, error) {
	value, sent := patch["message"]
	delete(patch, "message")
	if !sent {
		return "", nil
	}
	message, isText := value.(string)
	if !isText {
		return "", errMessageNotText
	}
	return message, nil
}

func applyPatch(current Revision, patch map[string]any) (patched, error) {
	var base any
	raw, err := json.Marshal(patched{Spec: current.Spec, Env: current.Env})
	if err == nil {
		err = json.Unmarshal(raw, &base)
	}
	if err == nil {
		raw, err = json.Marshal(MergePatch(base, patch))
	}
	if err != nil {
		return patched{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result patched
	return result, patchError(decoder.Decode(&result))
}

func patchError(err error) error {
	var typed *json.UnmarshalTypeError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &typed) && typed.Field == "env":
		return errEnvValue
	case errors.As(err, &typed):
		return apperror.Invalid(typed.Field + " must be " + kindOf(typed.Type)).WithField(typed.Field)
	}
	return apperror.Invalid("the patched config is not valid: " + err.Error()).WithHint(patchShape)
}

func kindOf(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	}
	return "a whole number"
}

func specFields() []string {
	t := reflect.TypeFor[Spec]()
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		names = append(names, name)
	}
	return names
}

func envChanges(before, after map[string]string) (map[string]string, []string) {
	set, unset := map[string]string{}, []string{}
	for name, value := range after {
		if old, had := before[name]; !had || old != value {
			set[name] = value
		}
	}
	for name := range before {
		if _, kept := after[name]; !kept {
			unset = append(unset, name)
		}
	}
	slices.Sort(unset)
	return set, unset
}

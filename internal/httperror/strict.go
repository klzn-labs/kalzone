package httperror

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func BindStrict(c *gin.Context, out any) bool {
	if err := DecodeStrict(c.Request.Body, out); err != nil {
		Abort(c, err)
		return false
	}
	return true
}

var (
	errTrailingData = errors.New("trailing data")
	errNullBody     = errors.New("null body")
)

func DecodeStrict(body io.Reader, out any) error {
	accepted := acceptedFields(out)
	raw, err := readOneValue(body)
	if err != nil {
		return decodeError(err, accepted)
	}
	if err := exactKeys(raw, accepted); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return decodeError(err, accepted)
	}
	return validationError(binding.Validator.ValidateStruct(out), out)
}

func readOneValue(body io.Reader) (json.RawMessage, error) {
	decoder := json.NewDecoder(body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errTrailingData
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, errNullBody
	}
	return raw, nil
}

func decodeError(err error, accepted []string) error {
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return withAccepted(apperror.Invalid("the request body is empty"), "send a JSON object", accepted)
	case errors.Is(err, errTrailingData):
		return withAccepted(apperror.Invalid("the request body has data after the JSON object"), "send one JSON object", accepted)
	case errors.Is(err, errNullBody):
		return notAnObject(accepted)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return apperror.Invalid("the request body is not valid JSON").WithHint("the JSON ends early; check for a missing closing bracket or quote")
	case errors.As(err, &syntax):
		return apperror.Invalid("the request body is not valid JSON").WithHint(fmt.Sprintf("check the JSON near byte %d", syntax.Offset))
	case errors.As(err, &typed):
		return typeError(typed, accepted)
	}
	if field, unknown := unknownField(err); unknown {
		return unknownFieldError(field, accepted)
	}
	return apperror.Invalid("the request body is not valid JSON: " + err.Error())
}

func unknownFieldError(field string, accepted []string) error {
	return withAccepted(apperror.Invalid(fmt.Sprintf("unknown field %q", field)).WithField(field), "remove "+field, accepted)
}

func exactKeys(raw json.RawMessage, accepted []string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if miscased(key, accepted) {
			return unknownFieldError(key, accepted)
		}
	}
	return nil
}

func miscased(key string, accepted []string) bool {
	sameIgnoringCase := func(name string) bool { return strings.EqualFold(name, key) }
	return !slices.Contains(accepted, key) && slices.ContainsFunc(accepted, sameIgnoringCase)
}

func typeError(typed *json.UnmarshalTypeError, accepted []string) error {
	if typed.Field == "" {
		return notAnObject(accepted)
	}
	return apperror.Invalid(typed.Field + " must be " + jsonKind(typed.Type)).WithField(typed.Field)
}

func notAnObject(accepted []string) error {
	return withAccepted(apperror.Invalid("the request body must be a JSON object"), "send a JSON object", accepted)
}

func withAccepted(e *apperror.AppError, hint string, accepted []string) *apperror.AppError {
	return e.WithHint(hint + "; this body accepts: " + strings.Join(accepted, ", ")).
		WithDetails(map[string]any{"accepted": accepted})
}

func unknownField(err error) (string, bool) {
	quoted, found := strings.CutPrefix(err.Error(), "json: unknown field ")
	if !found {
		return "", false
	}
	field, unquoteErr := strconv.Unquote(quoted)
	return field, unquoteErr == nil
}

func validationError(err error, out any) error {
	var failures validator.ValidationErrors
	if !errors.As(err, &failures) || len(failures) == 0 {
		return err
	}
	field := jsonName(out, failures[0].StructField())
	if failures[0].Tag() == "required" {
		return apperror.Invalid(field + " is required").WithField(field).WithHint("send " + field)
	}
	return apperror.Invalid(field + " is not valid: " + failures[0].Tag()).WithField(field)
}

func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	}
	return "a number"
}

func acceptedFields(out any) []string {
	t := structType(out)
	names := []string{}
	for i := 0; t != nil && i < t.NumField(); i++ {
		if name := tagName(t.Field(i)); name != "-" {
			names = append(names, name)
		}
	}
	return names
}

func jsonName(out any, goName string) string {
	t := structType(out)
	if t == nil {
		return goName
	}
	if field, found := t.FieldByName(goName); found {
		return tagName(field)
	}
	return goName
}

func structType(out any) reflect.Type {
	t := reflect.TypeOf(out)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	return t
}

func tagName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" {
		return field.Name
	}
	return name
}

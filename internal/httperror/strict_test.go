package httperror_test

import (
	"strings"
	"testing"

	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

type deployBody struct {
	Service  string   `json:"service" binding:"required"`
	Release  string   `json:"release" binding:"required"`
	Strategy string   `json:"strategy,omitempty"`
	Teams    []string `json:"teams,omitempty"`
}

func decode(raw string) (deployBody, *apperror.AppError) {
	var body deployBody
	err := httperror.DecodeStrict(strings.NewReader(raw), &body)
	if err == nil {
		return body, nil
	}
	return body, err.(*apperror.AppError)
}

func Test_DecodeStrict_AcceptsAValidBody(t *testing.T) {
	body, err := decode(`{"service":"api","release":"r1"}`)

	require.Nil(t, err)
	require.Equal(t, deployBody{Service: "api", Release: "r1"}, body)
}

func Test_DecodeStrict_NamesAnUnknownFieldAndWhatIsAccepted(t *testing.T) {
	_, err := decode(`{"service":"api","release_id":"r1"}`)

	require.Equal(t, apperror.CodeInvalidInput, err.Code)
	require.Equal(t, "release_id", err.Field)
	require.Equal(t, "remove release_id; this body accepts: service, release, strategy, teams", err.Hint)
	require.Equal(t, []string{"service", "release", "strategy", "teams"}, err.Details["accepted"])
}

func Test_DecodeStrict_NamesAMissingRequiredFieldByItsJSONName(t *testing.T) {
	_, err := decode(`{"service":"api"}`)

	require.Equal(t, "release", err.Field)
	require.Equal(t, "release is required", err.Message)
	require.Equal(t, "send release", err.Hint)
}

func Test_DecodeStrict_ExplainsBadJSON(t *testing.T) {
	_, wrongType := decode(`{"service":"api","release":"r1","teams":"places"}`)
	_, empty := decode(``)
	_, broken := decode(`{"service":`)

	require.Equal(t, "teams", wrongType.Field)
	require.Equal(t, "teams must be an array", wrongType.Message)
	require.Contains(t, empty.Hint, "this body accepts: service, release")
	require.Equal(t, apperror.CodeInvalidInput, broken.Code)
}

func Test_DecodeStrict_RefusesABodyThatIsNotAnObject(t *testing.T) {
	_, array := decode(`[]`)
	_, text := decode(`"x"`)

	for _, err := range []*apperror.AppError{array, text} {
		require.Equal(t, apperror.CodeInvalidInput, err.Code)
		require.Equal(t, "the request body must be a JSON object", err.Message)
		require.Equal(t, "send a JSON object; this body accepts: service, release, strategy, teams", err.Hint)
	}
}

func Test_DecodeStrict_TreatsTruncatedJSONLikeBrokenJSON(t *testing.T) {
	_, err := decode(`{"service":"api","rel`)

	require.Equal(t, "the request body is not valid JSON", err.Message)
	require.Contains(t, err.Hint, "ends early")
}

func Test_DecodeStrict_RefusesDataAfterTheObject(t *testing.T) {
	_, err := decode(`{"service":"api","release":"r1"} junk`)

	require.Equal(t, apperror.CodeInvalidInput, err.Code)
	require.Contains(t, err.Message, "after the JSON object")
	require.Contains(t, err.Hint, "this body accepts: service, release")
}

func Test_DecodeStrict_RefusesClosingTokensAfterTheObject(t *testing.T) {
	for _, raw := range []string{`{"service":"api","release":"r1"}}`, `{"service":"api","release":"r1"}]`} {
		_, err := decode(raw)

		require.NotNil(t, err, raw)
		require.Equal(t, apperror.CodeInvalidInput, err.Code)
		require.Contains(t, err.Message, "after the JSON object")
		require.Equal(t, "send one JSON object; this body accepts: service, release, strategy, teams", err.Hint)
	}
}

func Test_DecodeStrict_RefusesANullBody(t *testing.T) {
	_, err := decode(` null `)

	require.NotNil(t, err)
	require.Equal(t, apperror.CodeInvalidInput, err.Code)
	require.Equal(t, "the request body must be a JSON object", err.Message)
	require.Equal(t, "send a JSON object; this body accepts: service, release, strategy, teams", err.Hint)
}

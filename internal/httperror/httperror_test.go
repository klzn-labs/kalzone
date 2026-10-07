package httperror_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_Resolve(t *testing.T) {
	cases := map[error]int{
		apperror.Forbidden("x"):                            http.StatusForbidden,
		apperror.NotFound("x"):                             http.StatusNotFound,
		apperror.Invalid("x"):                              http.StatusBadRequest,
		apperror.Conflict("x"):                             http.StatusConflict,
		apperror.New(apperror.CodeUnauthenticated, "x"):    http.StatusUnauthorized,
		errors.Join(apperror.Invalid("x"), errors.New("")): http.StatusBadRequest,
		errors.New("boom"):                                 http.StatusInternalServerError,
	}
	for err, want := range cases {
		status, body := httperror.Resolve(err)
		require.Equal(t, want, status, err.Error())
		require.NotEmpty(t, body.Code)
	}
}

func Test_Resolve_HidesInternalMessages(t *testing.T) {
	_, body := httperror.Resolve(errors.New("dial tcp 10.0.0.3: secret detail"))

	require.Equal(t, "internal error", body.Message)
}

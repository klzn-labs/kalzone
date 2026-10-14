package httperror_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func Test_Resolve(t *testing.T) {
	cases := map[error]int{
		apperror.Forbidden("x"):                            http.StatusForbidden,
		apperror.NotFound("x"):                             http.StatusNotFound,
		apperror.Invalid("x"):                              http.StatusBadRequest,
		apperror.Conflict("x"):                             http.StatusConflict,
		apperror.Stale("x"):                                http.StatusConflict,
		apperror.New(apperror.CodeUnauthenticated, "x"):    http.StatusUnauthorized,
		apperror.New(apperror.CodeMediaType, "x"):          http.StatusUnsupportedMediaType,
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

func Test_Abort_AnswersAWaitingChangeWith202(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	pending := &approval.Pending{Kind: "change_request", ID: "c1", Status: "pending", Poll: "/api/v1/requests/c1"}

	httperror.Abort(c, fmt.Errorf("deploy: %w", pending))

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), `"kind":"change_request"`)
	require.Contains(t, w.Body.String(), `"poll":"/api/v1/requests/c1"`)
}

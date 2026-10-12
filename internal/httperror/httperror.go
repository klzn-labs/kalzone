package httperror

import (
	"errors"
	"log"
	"net/http"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/gin-gonic/gin"
)

var statuses = map[apperror.Code]int{
	apperror.CodeUnauthenticated:  http.StatusUnauthorized,
	apperror.CodeForbidden:        http.StatusForbidden,
	apperror.CodeInvalidInput:     http.StatusBadRequest,
	apperror.CodeNotFound:         http.StatusNotFound,
	apperror.CodeConflict:         http.StatusConflict,
	apperror.CodeLastMaintainer:   http.StatusConflict,
	apperror.CodeSignInFailed:     http.StatusUnauthorized,
	apperror.CodeInvalidCSRFToken: http.StatusForbidden,
	apperror.CodeUpstream:         http.StatusBadGateway,
	apperror.CodeNotConfigured:    http.StatusServiceUnavailable,
	apperror.CodeMissingScope:     http.StatusForbidden,
	apperror.CodeTeamNotGranted:   http.StatusForbidden,
	apperror.CodeApprovalRequired: http.StatusForbidden,
	apperror.CodeStale:            http.StatusConflict,
	apperror.CodeTokenInvalid:     http.StatusUnauthorized,
	apperror.CodeTokenExpired:     http.StatusUnauthorized,
	apperror.CodeTokenRevoked:     http.StatusUnauthorized,
}

var errInternal = apperror.New(apperror.CodeInternal, "internal error")

func Resolve(err error) (int, *apperror.AppError) {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		log.Printf("unexpected error: %v", err)
		return http.StatusInternalServerError, errInternal
	}
	status, known := statuses[appErr.Code]
	if !known {
		log.Printf("unmapped error code %q: %v", appErr.Code, err)
		return http.StatusInternalServerError, errInternal
	}
	return status, appErr
}

type Body struct {
	Error *apperror.AppError `json:"error"`
}

func Abort(c *gin.Context, err error) {
	var pending *approval.Pending
	if errors.As(err, &pending) {
		c.AbortWithStatusJSON(http.StatusAccepted, pending)
		return
	}
	status, body := Resolve(err)
	c.AbortWithStatusJSON(status, Body{Error: body})
}

func BindJSON(c *gin.Context, out any) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		Abort(c, apperror.Invalid("invalid request body: "+err.Error()))
		return false
	}
	return true
}

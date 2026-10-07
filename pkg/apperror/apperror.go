package apperror

import "errors"

type Code string

const (
	CodeInternal         Code = "err_internal"
	CodeUnauthenticated  Code = "err_unauthenticated"
	CodeForbidden        Code = "err_forbidden"
	CodeInvalidInput     Code = "err_invalid_input"
	CodeNotFound         Code = "err_not_found"
	CodeConflict         Code = "err_conflict"
	CodeLastMaintainer   Code = "err_last_maintainer"
	CodeSignInFailed     Code = "err_sign_in_failed"
	CodeInvalidCSRFToken Code = "err_invalid_csrf_token"
	CodeUpstream         Code = "err_upstream"
	CodeNotConfigured    Code = "err_not_configured"
)

type AppError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) Is(target error) bool {
	var other *AppError
	if !errors.As(target, &other) {
		return false
	}
	return e.Code == other.Code
}

func Invalid(message string) *AppError {
	return New(CodeInvalidInput, message)
}

func NotFound(message string) *AppError {
	return New(CodeNotFound, message)
}

func Conflict(message string) *AppError {
	return New(CodeConflict, message)
}

func Forbidden(message string) *AppError {
	return New(CodeForbidden, message)
}

func CodeOf(err error) Code {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return CodeInternal
	}
	return appErr.Code
}

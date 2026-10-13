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
	CodeTokenInvalid     Code = "err_token_invalid"
	CodeTokenExpired     Code = "err_token_expired"
	CodeTokenRevoked     Code = "err_token_revoked"
	CodeMissingScope     Code = "err_missing_scope"
	CodeTeamNotGranted   Code = "err_team_not_granted"
	CodeApprovalRequired Code = "err_approval_required"
	CodeStale            Code = "err_stale"
)

type AppError struct {
	Code          Code           `json:"code"`
	Message       string         `json:"message"`
	Hint          string         `json:"hint,omitempty"`
	Field         string         `json:"field,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
	RequiredScope string         `json:"required_scope,omitempty"`
	Team          string         `json:"team,omitempty"`
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) WithHint(hint string) *AppError {
	copied := *e
	copied.Hint = hint
	return &copied
}

func (e *AppError) WithField(field string) *AppError {
	copied := *e
	copied.Field = field
	return &copied
}

func (e *AppError) WithDetails(details map[string]any) *AppError {
	copied := *e
	copied.Details = details
	return &copied
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

func Stale(message string) *AppError {
	return New(CodeStale, message)
}

func Forbidden(message string) *AppError {
	return New(CodeForbidden, message)
}

func MissingScope(scopeName string) *AppError {
	return &AppError{
		Code: CodeMissingScope, Message: "this token lacks " + scopeName, RequiredScope: scopeName,
		Hint: "ask the token's owner for a token with the " + scopeName + " scope",
	}
}

func TeamNotGranted(teamID string) *AppError {
	return &AppError{
		Code: CodeTeamNotGranted, Message: "this token is not granted team " + teamID, Team: teamID,
		Hint: "this token is limited to other teams; ask for a token granted this team",
	}
}

func CodeOf(err error) Code {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return CodeInternal
	}
	return appErr.Code
}

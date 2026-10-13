package entities

import (
	"time"

	"github.com/adrianozp/platform/pkg/actor"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusExecuted  Status = "executed"
	StatusFailed    Status = "failed"
	StatusRejected  Status = "rejected"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

var Statuses = []Status{StatusPending, StatusApproved, StatusExecuted, StatusFailed, StatusRejected, StatusCancelled, StatusExpired}

const (
	Lifetime       = 24 * time.Hour
	ExecutionLimit = 10 * time.Minute
	SummaryLimit   = 255
	TextLimit      = 500
)

type Request struct {
	ID             string
	Kind           string
	Subject        string
	ServiceID      string
	ServiceSlug    string
	TeamID         string
	Environment    string
	Summary        string
	Payload        []byte
	RequestedBy    string
	RequesterLogin string
	TokenID        string
	TokenName      string
	Status         Status
	DecidedBy      string
	DeciderLogin   string
	DecidedAt      *time.Time
	Reason         string
	Result         string
	ErrorCode      string
	ErrorMessage   string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	FinishedAt     *time.Time
}

type Filter struct {
	Status   Status
	TeamIDs  []string
	Before   *time.Time
	BeforeID string
	Limit    int
}

func (r Request) Pending() bool {
	return r.Status == StatusPending
}

func (r Request) ExpiredAt(now time.Time) bool {
	return r.Pending() && !now.Before(r.ExpiresAt)
}

func (r Request) Approve(by actor.Actor, at time.Time) Request {
	r.Status, r.DecidedBy, r.DeciderLogin, r.DecidedAt = StatusApproved, by.UserID, by.Login, &at
	return r
}

func (r Request) Close(status Status, by actor.Actor, reason string, at time.Time) Request {
	r.Status, r.DecidedBy, r.DeciderLogin, r.DecidedAt = status, by.UserID, by.Login, &at
	r.Reason, r.FinishedAt = Clip(reason, TextLimit), &at
	return r
}

func (r Request) Succeed(result string, at time.Time) Request {
	r.Status, r.Result, r.FinishedAt = StatusExecuted, Clip(result, SummaryLimit), &at
	return r
}

func (r Request) Fail(code, message string, at time.Time) Request {
	r.Status, r.ErrorCode, r.ErrorMessage, r.FinishedAt = StatusFailed, code, Clip(message, TextLimit), &at
	return r
}

func Clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

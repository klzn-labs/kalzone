package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
)

type changeRequest struct {
	ID             string `gorm:"primaryKey"`
	Kind           string
	Subject        string
	ServiceID      string
	ServiceSlug    string
	Resource       string
	TeamID         string
	Environment    string
	Summary        string
	Payload        []byte
	RequestedBy    string
	RequesterLogin string
	TokenID        string
	TokenName      string
	Status         string
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

func (changeRequest) TableName() string { return "change_requests" }

func fromRequest(r entities.Request) changeRequest {
	return changeRequest{
		ID: r.ID, Kind: r.Kind, Subject: r.Subject, ServiceID: r.ServiceID, ServiceSlug: r.ServiceSlug, Resource: r.Resource,
		TeamID: r.TeamID, Environment: r.Environment, Summary: r.Summary, Payload: r.Payload,
		RequestedBy: r.RequestedBy, RequesterLogin: r.RequesterLogin, TokenID: r.TokenID, TokenName: r.TokenName,
		Status: string(r.Status), DecidedBy: r.DecidedBy, DeciderLogin: r.DeciderLogin, DecidedAt: r.DecidedAt,
		Reason: r.Reason, Result: r.Result, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, FinishedAt: r.FinishedAt,
	}
}

func (m changeRequest) toEntity() entities.Request {
	return entities.Request{
		ID: m.ID, Kind: m.Kind, Subject: m.Subject, ServiceID: m.ServiceID, ServiceSlug: m.ServiceSlug, Resource: m.Resource,
		TeamID: m.TeamID, Environment: m.Environment, Summary: m.Summary, Payload: m.Payload,
		RequestedBy: m.RequestedBy, RequesterLogin: m.RequesterLogin, TokenID: m.TokenID, TokenName: m.TokenName,
		Status: entities.Status(m.Status), DecidedBy: m.DecidedBy, DeciderLogin: m.DeciderLogin, DecidedAt: utc(m.DecidedAt),
		Reason: m.Reason, Result: m.Result, ErrorCode: m.ErrorCode, ErrorMessage: m.ErrorMessage,
		CreatedAt: m.CreatedAt.UTC(), ExpiresAt: m.ExpiresAt.UTC(), FinishedAt: utc(m.FinishedAt),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

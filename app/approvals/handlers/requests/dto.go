package requests

import (
	"time"

	"github.com/adrianozp/platform/app/approvals/entities"
)

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type requestDTO struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind" enum:"config.revise,secret.set,secret.unset,deploy,promote,rollback,config.apply,queue.create,queue.update,queue.delete"`
	Status      string     `json:"status" enum:"pending,approved,executed,failed,rejected,cancelled,expired"`
	Service     string     `json:"service"`
	Environment string     `json:"environment"`
	Summary     string     `json:"summary"`
	RequestedBy string     `json:"requested_by"`
	Token       string     `json:"token"`
	ApproveURL  string     `json:"approve_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	DecidedBy   string     `json:"decided_by,omitempty"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Result      string     `json:"result,omitempty"`
	Error       *errorDTO  `json:"error,omitempty"`
}

func toDTO(r entities.Request, baseURL string) requestDTO {
	dto := requestDTO{
		ID: r.ID, Kind: r.Kind, Status: string(r.Status), Service: r.ServiceSlug, Environment: r.Environment,
		Summary: r.Summary, RequestedBy: r.RequesterLogin, Token: r.TokenName, ApproveURL: baseURL + r.ApprovePath(),
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, DecidedBy: r.DeciderLogin, DecidedAt: r.DecidedAt,
		Reason: r.Reason, Result: r.Result,
	}
	if r.ErrorCode != "" {
		dto.Error = &errorDTO{Code: r.ErrorCode, Message: r.ErrorMessage}
	}
	return dto
}

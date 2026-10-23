package crons

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind        string  `json:"kind"`
	Environment string  `json:"environment"`
	Name        string  `json:"name"`
	Draft       *Draft  `json:"draft,omitempty"`
	Change      *Change `json:"change,omitempty"`
}

const previewedPayload = 200

var (
	summaries          = map[string]string{approval.KindCronCreate: "create cron ", approval.KindCronUpdate: "change cron ", approval.KindCronDelete: "delete cron "}
	errUnknownProposal = apperror.Invalid("this request cannot be executed")
	errNotApproved     = apperror.Forbidden("only an approved request can apply this change")
)

func (u CronsUsecase) propose(ctx context.Context, a actor.Actor, c entities.Cron, o Proposal) error {
	service, err := u.catalog.FindServiceByID(ctx, c.ServiceID)
	if err != nil {
		return err
	}
	o.Environment, o.Name = c.Environment, c.Name
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: o.Kind, Subject: "cron." + c.Name, ServiceID: c.ServiceID, ServiceSlug: service.Slug,
		TeamID: c.TeamID, Environment: c.Environment, Summary: summaries[o.Kind] + c.Name, Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func (u CronsUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Cron, error) {
	if err := u.approved(ctx, a, o); err != nil {
		return entities.Cron{}, err
	}
	return u.replay(ctx, a, o)
}

func (u CronsUsecase) approved(ctx context.Context, a actor.Actor, o Proposal) error {
	gate, err := u.gateOf(ctx, a, o)
	if err != nil {
		return err
	}
	if gate != authz.Approved && gate != authz.Open {
		return errNotApproved
	}
	return nil
}

func (u CronsUsecase) gateOf(ctx context.Context, a actor.Actor, o Proposal) (authz.Gate, error) {
	if o.Kind == approval.KindCronCreate && o.Draft != nil {
		service, err := u.catalog.FindService(ctx, o.Draft.Service)
		if err != nil {
			return "", err
		}
		return u.authz.Gate(ctx, a, authz.EditCron, authz.Resource{TeamID: service.OwnerTeamID, Environment: o.Draft.Environment})
	}
	_, gate, err := u.editable(ctx, a, o.Environment, o.Name)
	return gate, err
}

func (u CronsUsecase) replay(ctx context.Context, a actor.Actor, o Proposal) (entities.Cron, error) {
	switch {
	case o.Kind == approval.KindCronCreate && o.Draft != nil:
		return u.Create(ctx, a, *o.Draft)
	case o.Kind == approval.KindCronUpdate && o.Change != nil:
		return u.Update(ctx, a, o.Environment, o.Name, *o.Change)
	case o.Kind == approval.KindCronDelete:
		return entities.Cron{}, u.Delete(ctx, a, o.Environment, o.Name)
	}
	return entities.Cron{}, errUnknownProposal
}

func (u CronsUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	switch {
	case o.Kind == approval.KindCronCreate && o.Draft != nil:
		return previewCreate(*o.Draft), nil
	case o.Kind == approval.KindCronUpdate && o.Change != nil:
		return u.previewUpdate(ctx, o)
	case o.Kind == approval.KindCronDelete:
		return previewDelete(o), nil
	}
	return approval.Preview{}, errUnknownProposal
}

func previewCreate(d Draft) approval.Preview {
	planned := entities.Cron{Schedule: d.Schedule, Path: d.Path, Payload: d.Payload, Timeout: timeoutOr(d.Timeout)}
	lines := []approval.Line{{Key: "cron", After: d.Name}, {Key: "service", After: d.Service}}
	return approval.Preview{Lines: append(lines, fieldLines(nil, fieldsOf(planned))...)}
}

func (u CronsUsecase) previewUpdate(ctx context.Context, o Proposal) (approval.Preview, error) {
	c, err := u.store.Get(ctx, o.Environment, o.Name)
	if err != nil {
		return approval.Preview{}, err
	}
	next, err := applied(c, *o.Change)
	if err != nil {
		return approval.Preview{Conflict: "approving will fail: " + err.Error()}, nil
	}
	return approval.Preview{Lines: fieldLines(fieldsOf(c), fieldsOf(next))}, nil
}

func previewDelete(o Proposal) approval.Preview {
	return approval.Preview{Lines: []approval.Line{
		{Key: "cron", Before: o.Name, After: "deleted"}, {Key: "runs", After: "history dropped"},
	}}
}

func fieldsOf(c entities.Cron) [][2]string {
	return [][2]string{
		{"schedule", c.Schedule}, {"path", c.Path}, {"timeout", c.Timeout.String()}, {"payload", payloadPreview(c.Body())},
	}
}

func payloadPreview(body []byte) string {
	runes := []rune(string(body))
	if len(runes) <= previewedPayload {
		return string(runes)
	}
	return string(runes[:previewedPayload]) + "…"
}

func fieldLines(before, after [][2]string) []approval.Line {
	lines := []approval.Line{}
	for i, field := range after {
		old := ""
		if before != nil {
			old = before[i][1]
		}
		if old != field[1] {
			lines = append(lines, approval.Line{Key: field[0], Before: old, After: field[1]})
		}
	}
	return lines
}

package queues

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
)

type Proposal struct {
	Kind        string  `json:"kind"`
	Environment string  `json:"environment"`
	Name        string  `json:"name"`
	Draft       *Draft  `json:"draft,omitempty"`
	Change      *Change `json:"change,omitempty"`
	Force       bool    `json:"force,omitempty"`
}

var (
	summaries          = map[string]string{approval.KindQueueCreate: "create queue ", approval.KindQueueUpdate: "change queue ", approval.KindQueueDelete: "delete queue "}
	errUnknownProposal = apperror.Invalid("this request cannot be executed")
	errNotApproved     = apperror.Forbidden("only an approved request can apply this change")
)

func (u QueuesUsecase) propose(ctx context.Context, a actor.Actor, q entities.Queue, o Proposal) error {
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if err != nil {
		return err
	}
	o.Environment, o.Name = q.Environment, q.Name
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: o.Kind, Subject: "queue." + q.Name, ServiceID: q.ConsumerID, ServiceSlug: consumer.Slug,
		TeamID: q.TeamID, Environment: q.Environment, Summary: summaryOf(o, q.Name), Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func summaryOf(o Proposal, name string) string {
	summary := summaries[o.Kind] + name
	if o.Force {
		summary += " (force)"
	}
	return summary
}

func (u QueuesUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Queue, error) {
	if err := u.approved(ctx, a, o); err != nil {
		return entities.Queue{}, err
	}
	return u.replay(ctx, a, o)
}

func (u QueuesUsecase) approved(ctx context.Context, a actor.Actor, o Proposal) error {
	gate, err := u.gateOf(ctx, a, o)
	if err != nil {
		return err
	}
	if gate != authz.Approved && gate != authz.Open {
		return errNotApproved
	}
	return nil
}

func (u QueuesUsecase) gateOf(ctx context.Context, a actor.Actor, o Proposal) (authz.Gate, error) {
	if o.Kind == approval.KindQueueCreate && o.Draft != nil {
		consumer, err := u.services.FindService(ctx, o.Draft.Consumer)
		if err != nil {
			return "", err
		}
		return u.authz.Gate(ctx, a, authz.EditQueue, authz.Resource{TeamID: consumer.OwnerTeamID, Environment: o.Draft.Environment})
	}
	_, gate, err := u.editable(ctx, a, o.Environment, o.Name)
	return gate, err
}

func (u QueuesUsecase) replay(ctx context.Context, a actor.Actor, o Proposal) (entities.Queue, error) {
	switch {
	case o.Kind == approval.KindQueueCreate && o.Draft != nil:
		return u.Create(ctx, a, *o.Draft)
	case o.Kind == approval.KindQueueUpdate && o.Change != nil:
		return u.Update(ctx, a, o.Environment, o.Name, *o.Change)
	case o.Kind == approval.KindQueueDelete:
		return entities.Queue{}, u.Delete(ctx, a, o.Environment, o.Name, o.Force)
	}
	return entities.Queue{}, errUnknownProposal
}

func (u QueuesUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	switch {
	case o.Kind == approval.KindQueueCreate && o.Draft != nil:
		return u.previewCreate(ctx, *o.Draft)
	case o.Kind == approval.KindQueueUpdate && o.Change != nil:
		return u.previewUpdate(ctx, o)
	}
	return previewDelete(o), nil
}

func previewDelete(o Proposal) approval.Preview {
	lines := []approval.Line{{Key: "queue", Before: o.Name, After: "deleted"}}
	if o.Force {
		lines = append(lines, approval.Line{Key: "force", After: "queued messages will be dropped"})
	}
	return approval.Preview{Lines: lines}
}

func (u QueuesUsecase) previewCreate(ctx context.Context, d Draft) (approval.Preview, error) {
	consumer, err := u.services.FindService(ctx, d.Consumer)
	if err != nil {
		return approval.Preview{}, err
	}
	ids, err := u.publisherIDs(ctx, consumer, d.Publishers)
	if err != nil {
		return approval.Preview{}, err
	}
	names, err := u.slugsOf(ctx, ids)
	if err != nil {
		return approval.Preview{}, err
	}
	lines := settingsLines(nil, settingFields(withDefaults(d.Settings)))
	return approval.Preview{Lines: append(lines, approval.Line{Key: "publishers", After: names})}, nil
}

func (u QueuesUsecase) slugsOf(ctx context.Context, ids []string) (string, error) {
	slugs := make([]string, 0, len(ids))
	for _, id := range ids {
		s, err := u.services.FindServiceByID(ctx, id)
		if err != nil {
			return "", err
		}
		slugs = append(slugs, s.Slug)
	}
	return strings.Join(slugs, ", "), nil
}

func (u QueuesUsecase) previewUpdate(ctx context.Context, o Proposal) (approval.Preview, error) {
	q, err := u.store.Get(ctx, o.Environment, o.Name)
	if err != nil {
		return approval.Preview{}, err
	}
	after := q.Settings
	if o.Change.Settings != nil {
		after = *o.Change.Settings
	}
	preview := approval.Preview{Lines: settingsLines(settingFields(q.Settings), settingFields(after))}
	if err := u.addPublishers(ctx, &preview, q, o.Change.Publishers); err != nil {
		return approval.Preview{}, err
	}
	if q.Version != o.Change.BaseVersion {
		preview.Conflict = fmt.Sprintf("approving will fail: the queue moved from version %d to %d since the request", o.Change.BaseVersion, q.Version)
	}
	return preview, nil
}

func (u QueuesUsecase) addPublishers(ctx context.Context, preview *approval.Preview, q entities.Queue, requested *[]string) error {
	if requested == nil {
		return nil
	}
	before, after, err := u.publisherSlugs(ctx, q, *requested)
	if err != nil {
		return err
	}
	preview.Lines = append(preview.Lines, approval.Line{Key: "publishers", Before: before, After: after})
	return nil
}

func (u QueuesUsecase) publisherSlugs(ctx context.Context, q entities.Queue, requested []string) (string, string, error) {
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if err != nil {
		return "", "", err
	}
	ids, err := u.publisherIDs(ctx, consumer, requested)
	if err != nil {
		return "", "", err
	}
	before, err := u.slugsOf(ctx, q.Publishers)
	if err != nil {
		return "", "", err
	}
	after, err := u.slugsOf(ctx, ids)
	return before, after, err
}

func settingFields(s entities.Settings) [][2]string {
	return [][2]string{
		{"path", s.Path}, {"rate_per_second", strconv.FormatFloat(s.RatePerSecond, 'g', -1, 64)},
		{"max_in_flight", strconv.Itoa(s.MaxInFlight)}, {"max_attempts", strconv.Itoa(s.MaxAttempts)},
		{"backoff", fmt.Sprintf("%s %s to %s", s.Backoff.Kind, s.Backoff.Initial, s.Backoff.Max)},
		{"delivery_timeout", s.DeliveryTimeout.String()}, {"retention", s.Retention.String()},
		{"max_message_bytes", strconv.Itoa(s.MaxMessageBytes)},
	}
}

func settingsLines(before, after [][2]string) []approval.Line {
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

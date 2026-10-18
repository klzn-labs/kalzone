package configs

import (
	"context"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind        string             `json:"kind"`
	Service     string             `json:"service"`
	Environment string             `json:"environment"`
	Base        int                `json:"base"`
	Change      entities.ChangeSet `json:"change"`
}

var errNotApproved = apperror.Forbidden("only an approved request can apply this change")

var verbs = map[string]string{approval.KindConfigRevise: "change", approval.KindSecretSet: "set", approval.KindSecretUnset: "remove"}

func (u ConfigUsecase) propose(ctx context.Context, a actor.Actor, e edit, kind string, cs entities.ChangeSet, changes []entities.Change) error {
	keys := entities.ChangedKeys(changes)
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: kind, Subject: subjectOf(kind, keys), ServiceID: e.service.ID, ServiceSlug: e.service.Slug,
		TeamID: e.service.OwnerTeamID, Environment: e.environment, Summary: verbs[kind] + " " + strings.Join(keys, ", "),
		Proposal: Proposal{Kind: kind, Service: e.service.Slug, Environment: e.environment, Base: e.current.Number, Change: cs},
	})
	if err != nil {
		return err
	}
	return pending
}

func subjectOf(kind string, keys []string) string {
	if kind == approval.KindConfigRevise {
		return "config"
	}
	return keys[0]
}

func (u ConfigUsecase) ApplyApproved(ctx context.Context, a actor.Actor, p Proposal) (entities.Revision, error) {
	e, err := u.editable(ctx, a, actionOf(p.Kind), p.Service, p.Environment)
	if err != nil {
		return entities.Revision{}, err
	}
	if e.gate != authz.Approved && e.gate != authz.Open {
		return entities.Revision{}, errNotApproved
	}
	if err := u.unmoved(ctx, e, p); err != nil {
		return entities.Revision{}, err
	}
	return u.change(ctx, a, e, p.Kind, p.Change)
}

func actionOf(kind string) authz.Action {
	if kind == approval.KindConfigRevise {
		return authz.EditConfig
	}
	return authz.EditSecrets
}

func (u ConfigUsecase) unmoved(ctx context.Context, e edit, p Proposal) error {
	conflicts, err := u.conflicts(ctx, e.target, e.current, p)
	if err != nil || len(conflicts) == 0 {
		return err
	}
	return apperror.Stale("changed since the request: " + strings.Join(conflicts, ", "))
}

func (u ConfigUsecase) conflicts(ctx context.Context, t target, current entities.Revision, p Proposal) ([]string, error) {
	base, err := u.revision(ctx, t.service.ID, t.environment, p.Base)
	if err != nil {
		return nil, err
	}
	return entities.Conflicts(p.Change, base, current), nil
}

func (u ConfigUsecase) Preview(ctx context.Context, p Proposal) (approval.Preview, error) {
	t, err := u.resolve(ctx, p.Service, p.Environment)
	if err != nil {
		return approval.Preview{}, err
	}
	current, err := u.current(ctx, t)
	if err != nil {
		return approval.Preview{}, err
	}
	conflicts, err := u.conflicts(ctx, t, current, p)
	return previewOf(current, p, conflicts), err
}

func previewOf(current entities.Revision, p Proposal, conflicts []string) approval.Preview {
	preview := approval.Preview{Lines: linesOf(entities.Diff(current, p.Change.ApplyTo(current.Next())))}
	if current.Number != p.Base {
		preview.Moved = fmt.Sprintf("the config moved from revision %d to %d since the request", p.Base, current.Number)
	}
	if len(conflicts) > 0 {
		preview.Conflict = "approving will fail: " + strings.Join(conflicts, ", ") + " changed since the request"
	}
	return preview
}

func linesOf(changes []entities.Change) []approval.Line {
	lines := make([]approval.Line, 0, len(changes))
	for _, c := range changes {
		lines = append(lines, lineOf(c))
	}
	return lines
}

func lineOf(c entities.Change) approval.Line {
	line := approval.Line{Key: c.Section + "." + c.Key, Before: c.Before, After: c.After}
	if c.Section == "secret" {
		line.Before, line.After = secretStates(c.Kind)
	}
	return line
}

func secretStates(kind entities.ChangeKind) (string, string) {
	switch kind {
	case entities.Added:
		return "", "new value"
	case entities.Removed:
		return "set", "removed"
	}
	return "set", "new value"
}

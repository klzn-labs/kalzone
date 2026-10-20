package databases

import (
	"context"
	"strconv"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Proposal struct {
	Kind        string `json:"kind"`
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Draft       *Draft `json:"draft,omitempty"`
	Service     string `json:"service,omitempty"`
}

var (
	errUnknownProposal = apperror.Invalid("this request cannot be executed")
	errNotApproved     = apperror.Forbidden("only an approved request can apply this change")
)

func (u DatabasesUsecase) propose(ctx context.Context, a actor.Actor, db entities.Database, o Proposal) error {
	o.Environment, o.Name = db.Environment, db.Name
	pending, err := u.requests.Submit(ctx, a, approval.Change{
		Kind: o.Kind, Subject: subjectOf(o), Resource: "database " + db.Name, TeamID: db.TeamID,
		Environment: db.Environment, Summary: summaryFor(o, db), Proposal: o,
	})
	if err != nil {
		return err
	}
	return pending
}

func subjectOf(o Proposal) string {
	if o.Service == "" {
		return "database." + o.Name
	}
	return "database." + o.Name + "." + o.Service
}

func summaryFor(o Proposal, db entities.Database) string {
	if o.Kind == approval.KindDatabaseDelete && db.Adopted {
		return "release database " + o.Name
	}
	return summaryOf(o)
}

func summaryOf(o Proposal) string {
	switch o.Kind {
	case approval.KindDatabaseCreate:
		return "create database " + o.Name
	case approval.KindDatabaseAttach:
		return "attach database " + o.Name + " to " + o.Service
	case approval.KindDatabaseDetach:
		return "detach database " + o.Name + " from " + o.Service
	}
	return "delete database " + o.Name
}

func (u DatabasesUsecase) ApplyApproved(ctx context.Context, a actor.Actor, o Proposal) (entities.Database, error) {
	if err := u.approved(ctx, a, o); err != nil {
		return entities.Database{}, err
	}
	return u.replay(ctx, a, o)
}

func (u DatabasesUsecase) approved(ctx context.Context, a actor.Actor, o Proposal) error {
	gate, err := u.gateOf(ctx, a, o)
	if err != nil {
		return err
	}
	if gate != authz.Approved && gate != authz.Open {
		return errNotApproved
	}
	return nil
}

func (u DatabasesUsecase) gateOf(ctx context.Context, a actor.Actor, o Proposal) (authz.Gate, error) {
	if o.Kind == approval.KindDatabaseCreate && o.Draft != nil {
		teamID, err := u.catalog.FindTeam(ctx, o.Draft.Team)
		if err != nil {
			return "", err
		}
		return u.authz.Gate(ctx, a, authz.EditDatabase, authz.Resource{TeamID: teamID, Environment: o.Draft.Environment})
	}
	_, gate, err := u.editable(ctx, a, o.Environment, o.Name)
	return gate, err
}

func (u DatabasesUsecase) replay(ctx context.Context, a actor.Actor, o Proposal) (entities.Database, error) {
	switch {
	case o.Kind == approval.KindDatabaseCreate && o.Draft != nil:
		return u.Create(ctx, a, *o.Draft)
	case o.Kind == approval.KindDatabaseAttach:
		return u.Attach(ctx, a, o.Environment, o.Name, o.Service)
	case o.Kind == approval.KindDatabaseDetach:
		return u.Detach(ctx, a, o.Environment, o.Name, o.Service)
	case o.Kind == approval.KindDatabaseDelete:
		return entities.Database{}, u.Delete(ctx, a, o.Environment, o.Name)
	}
	return entities.Database{}, errUnknownProposal
}

func (u DatabasesUsecase) Preview(ctx context.Context, o Proposal) (approval.Preview, error) {
	switch {
	case o.Kind == approval.KindDatabaseCreate && o.Draft != nil:
		return u.previewCreate(ctx, withDefaults(*o.Draft))
	case o.Kind == approval.KindDatabaseAttach:
		return previewAttach(o, approval.Line{Key: "service", After: o.Service}), nil
	case o.Kind == approval.KindDatabaseDetach:
		return previewAttach(o, approval.Line{Key: "service", Before: o.Service, After: "detached"}), nil
	case o.Kind == approval.KindDatabaseDelete:
		return u.previewDelete(ctx, o)
	}
	return approval.Preview{}, errUnknownProposal
}

func (u DatabasesUsecase) previewCreate(ctx context.Context, d Draft) (approval.Preview, error) {
	schema, err := u.schemaOf(ctx, d)
	if err != nil {
		return approval.Preview{}, err
	}
	return approval.Preview{Lines: []approval.Line{
		{Key: "database", After: d.Name}, {Key: "schema", After: schema},
		{Key: "team", After: d.Team}, {Key: "max_connections", After: strconv.Itoa(d.MaxConnections)},
	}}, nil
}

func previewAttach(o Proposal, service approval.Line) approval.Preview {
	return approval.Preview{Lines: []approval.Line{
		{Key: "database", Before: o.Name, After: o.Name}, service, {Key: "takes_effect", After: "next deploy"},
	}}
}

func (u DatabasesUsecase) previewDelete(ctx context.Context, o Proposal) (approval.Preview, error) {
	db, err := u.store.Get(ctx, o.Environment, o.Name)
	if err != nil {
		return approval.Preview{}, err
	}
	team, err := u.catalog.TeamSlug(ctx, db.TeamID)
	if err != nil {
		return approval.Preview{}, err
	}
	database, schema := removalOf(db)
	return approval.Preview{Lines: []approval.Line{
		{Key: "database", Before: db.Name, After: database}, {Key: "schema", Before: db.Schema, After: schema},
		{Key: "team", Before: team},
	}}, nil
}

func removalOf(db entities.Database) (string, string) {
	if db.Adopted {
		return "released", "kept"
	}
	return "deleted", "dropped"
}

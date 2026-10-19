package database

import (
	"context"
	"encoding/json"

	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Databases interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o databases.Proposal) (entities.Database, error)
	Preview(ctx context.Context, o databases.Proposal) (approval.Preview, error)
}

type Executor struct {
	databases Databases
}

func NewExecutor(databases Databases) Executor {
	return Executor{databases: databases}
}

var results = map[string]string{
	approval.KindDatabaseCreate: "database created",
	approval.KindDatabaseAttach: "database attached; takes effect on the service's next deploy",
	approval.KindDatabaseDetach: "database detached; takes effect on the service's next deploy",
	approval.KindDatabaseDelete: "database deleted",
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o databases.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	_, err := e.databases.ApplyApproved(ctx, a, o)
	return results[o.Kind], err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o databases.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.databases.Preview(ctx, o)
}

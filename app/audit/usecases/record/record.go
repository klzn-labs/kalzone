package record

import (
	"context"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/uuid"
)

type EventStore interface {
	Insert(ctx context.Context, e entities.Event) error
}

type RecordUsecase struct {
	store EventStore
}

func NewRecordUsecase(store EventStore) RecordUsecase {
	return RecordUsecase{store: store}
}

func (u RecordUsecase) Handle(ctx context.Context, e events.Event) error {
	return u.store.Insert(ctx, entities.Event{
		ID: uuid.NewUUID(), At: e.At, ActorID: e.ActorID, ActorLogin: e.ActorLogin, Action: e.Action,
		TargetKind: e.TargetKind, TargetID: e.TargetID, TargetName: e.TargetName,
		TeamID: e.TeamID, Environment: e.Environment, Details: e.Details,
	})
}

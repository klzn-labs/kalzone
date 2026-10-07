package record_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/audit/usecases/record"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

type store struct{ got []entities.Event }

func (s *store) Insert(_ context.Context, e entities.Event) error {
	s.got = append(s.got, e)
	return nil
}

func Test_Handle_RecordsTheEventWithAnID(t *testing.T) {
	s := &store{}
	at := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

	err := record.NewRecordUsecase(s).Handle(context.Background(), events.Event{
		At: at, ActorID: "u1", ActorLogin: "ana", Action: "service.created",
		TargetKind: "service", TargetID: "s1", TargetName: "api", TeamID: "t1", Environment: "dev",
	})

	require.NoError(t, err)
	require.NotEmpty(t, s.got[0].ID)
	require.Equal(t, at, s.got[0].At)
	require.Equal(t, "t1", s.got[0].TeamID)
	require.Equal(t, "api", s.got[0].TargetName)
}

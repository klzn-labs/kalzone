package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/stretchr/testify/require"
)

var at = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func Test_Request_ExpiresOnlyWhilePending(t *testing.T) {
	r := entities.Request{Status: entities.StatusPending, ExpiresAt: at}

	require.False(t, r.ExpiredAt(at.Add(-time.Second)))
	require.True(t, r.ExpiredAt(at))
	r.Status = entities.StatusApproved
	require.False(t, r.ExpiredAt(at.Add(time.Hour)))
}

func Test_Request_TransitionsRecordWhoAndWhen(t *testing.T) {
	mia := actor.Actor{UserID: "u-mia", Login: "mia"}
	r := entities.Request{Status: entities.StatusPending}

	approved := r.Approve(mia, at)
	rejected := r.Close(entities.StatusRejected, mia, "not now", at)
	done := approved.Succeed("revision 4", at)
	failed := approved.Fail("err_stale", "env.A changed", at)

	require.Equal(t, entities.StatusApproved, approved.Status)
	require.Equal(t, "mia", approved.DeciderLogin)
	require.Nil(t, approved.FinishedAt)
	require.Equal(t, "not now", rejected.Reason)
	require.Equal(t, &at, rejected.FinishedAt)
	require.Equal(t, "revision 4", done.Result)
	require.Equal(t, "err_stale", failed.ErrorCode)
	require.Equal(t, "mia", failed.DeciderLogin)
}

func Test_Clip_KeepsShortTextAndCutsLongText(t *testing.T) {
	require.Equal(t, "short", entities.Clip("short", 10))
	require.Equal(t, strings.Repeat("a", 9)+"…", entities.Clip(strings.Repeat("a", 20), 10))
}

func Test_Request_IsFinalOnceNothingMoreHappensToIt(t *testing.T) {
	final := map[entities.Status]bool{
		entities.StatusPending: false, entities.StatusApproved: false, entities.StatusExecuted: true, entities.StatusFailed: true,
		entities.StatusRejected: true, entities.StatusCancelled: true, entities.StatusExpired: true,
	}
	for _, status := range entities.Statuses {
		require.Equal(t, final[status], entities.Request{Status: status}.Final(), status)
	}
}

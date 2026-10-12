package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/app/approvals/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var (
	ctx     = context.Background()
	created = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	mia     = actor.Actor{UserID: "u-mia", Login: "mia"}
)

func request(id, team, subject string) entities.Request {
	return entities.Request{
		ID: id, Kind: "deploy", Subject: subject, ServiceID: "s1", ServiceSlug: "api", TeamID: team,
		Environment: "prod", Summary: "deploy release #2 to prod", Payload: []byte(`{"release_id": "r2"}`),
		RequestedBy: "u-bob", RequesterLogin: "bob", TokenID: "k1", TokenName: "claude-local",
		Status: entities.StatusPending, CreatedAt: created, ExpiresAt: created.Add(entities.Lifetime),
	}
}

func Test_Requests_RoundTrip(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))

	got, err := repo.Get(ctx, "c1")
	_, missing := repo.Get(ctx, "nope")

	require.NoError(t, err)
	require.JSONEq(t, `{"release_id":"r2"}`, string(got.Payload))
	got.Payload = request("c1", "t1", "deploy").Payload
	require.Equal(t, request("c1", "t1", "deploy"), got)
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_Requests_ListFiltersByStatusAndTeams(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))
	require.NoError(t, repo.Create(ctx, request("c2", "t2", "deploy")))
	require.NoError(t, repo.Transition(ctx, request("c2", "t2", "deploy").Close(entities.StatusRejected, mia, "no", created), entities.StatusPending))

	pending, _ := repo.List(ctx, entities.Filter{Status: entities.StatusPending, Limit: 10})
	t2, _ := repo.List(ctx, entities.Filter{TeamIDs: []string{"t2"}, Limit: 10})
	all, _ := repo.List(ctx, entities.Filter{Limit: 10})

	require.Len(t, pending, 1)
	require.Equal(t, "c1", pending[0].ID)
	require.Len(t, t2, 1)
	require.Equal(t, "no", t2[0].Reason)
	require.Len(t, all, 2)
}

func Test_Requests_TransitionOnlyFromTheExpectedStatus(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	r := request("c1", "t1", "deploy")
	require.NoError(t, repo.Create(ctx, r))

	first := repo.Transition(ctx, r.Approve(mia, created), entities.StatusPending)
	second := repo.Transition(ctx, r.Approve(mia, created), entities.StatusPending)
	got, _ := repo.Get(ctx, "c1")

	require.NoError(t, first)
	require.ErrorIs(t, second, apperror.Conflict(""))
	require.Equal(t, entities.StatusApproved, got.Status)
	require.Equal(t, "mia", got.DeciderLogin)
}

func Test_Requests_PendingForTheSameSubject(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, request("c1", "t1", "deploy")))
	require.NoError(t, repo.Create(ctx, request("c2", "t1", "secret.TOKEN")))

	same, err := repo.PendingFor(ctx, "s1", "prod", "deploy")

	require.NoError(t, err)
	require.Len(t, same, 1)
	require.Equal(t, "c1", same[0].ID)
}

func Test_Requests_OverdueAreExpiredPendingAndStuckApproved(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	now := created.Add(entities.Lifetime)
	stuck := request("c2", "t1", "deploy").Approve(mia, now.Add(-entities.ExecutionLimit))
	fresh := request("c3", "t1", "deploy").Approve(mia, now)
	for _, r := range []entities.Request{request("c1", "t1", "deploy"), stuck, fresh} {
		require.NoError(t, repo.Create(ctx, r))
	}

	overdue, err := repo.Overdue(ctx, now)

	require.NoError(t, err)
	require.Len(t, overdue, 2)
	require.Equal(t, []string{"c1", "c2"}, []string{overdue[0].ID, overdue[1].ID})
}

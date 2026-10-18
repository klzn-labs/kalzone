package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/approvals/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/page"
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

func Test_Requests_ListContinuesAfterACreationTimeAndID(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	for _, id := range []string{"c1", "c2", "c3"} {
		require.NoError(t, repo.Create(ctx, request(id, "t1", "deploy")))
	}

	first, err := repo.List(ctx, entities.Filter{Limit: 2})
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{Before: &first[1].CreatedAt, BeforeID: first[1].ID, Limit: 2})
	require.NoError(t, err)

	require.Equal(t, []string{"c3", "c2"}, []string{first[0].ID, first[1].ID})
	require.Len(t, rest, 1)
	require.Equal(t, "c1", rest[0].ID)
}

func createdAt(id string, at time.Time) entities.Request {
	r := request(id, "t1", "deploy")
	r.CreatedAt, r.ExpiresAt = at, at.Add(entities.Lifetime)
	return r
}

func Test_Requests_PagesAcrossRequestsStoredInTheSameMillisecond(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	base := created.Add(123 * time.Millisecond)
	offsets := map[string]time.Duration{"c0": -time.Millisecond, "c1": 400 * time.Microsecond, "c2": 100 * time.Microsecond, "c3": 300 * time.Microsecond}
	for id, offset := range offsets {
		require.NoError(t, repo.Create(ctx, createdAt(id, base.Add(offset))))
	}

	first, err := repo.List(ctx, entities.Filter{Limit: 2})
	require.NoError(t, err)
	next, err := page.Parse("2", page.Encode(page.TimeKey(first[1].CreatedAt, first[1].ID)))
	require.NoError(t, err)
	before, beforeID, err := next.Time()
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{Before: before, BeforeID: beforeID, Limit: 3})
	require.NoError(t, err)

	require.Equal(t, []string{"c3", "c2"}, []string{first[0].ID, first[1].ID})
	require.Equal(t, []string{"c1", "c0"}, []string{rest[0].ID, rest[1].ID})
}

func Test_Requests_ListBeforeAnInstantWithoutAnID(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, createdAt("c1", created)))
	require.NoError(t, repo.Create(ctx, createdAt("c2", created.Add(time.Second))))
	require.NoError(t, repo.Create(ctx, createdAt("c3", created.Add(time.Second))))
	instant := created.Add(time.Second)

	older, err := repo.List(ctx, entities.Filter{Before: &instant, Limit: 10})

	require.NoError(t, err)
	require.Len(t, older, 1)
	require.Equal(t, "c1", older[0].ID)
}

func Test_Requests_KeysetPagesStayWithinTheTeams(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	for _, r := range []entities.Request{request("c1", "t1", "deploy"), request("c2", "t2", "deploy"), request("c3", "t1", "deploy"), request("c4", "t2", "deploy"), request("c5", "t1", "deploy")} {
		require.NoError(t, repo.Create(ctx, r))
	}
	teams := []string{"t1", "t3"}

	first, err := repo.List(ctx, entities.Filter{TeamIDs: teams, Limit: 2})
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{TeamIDs: teams, Before: &first[1].CreatedAt, BeforeID: first[1].ID, Limit: 5})
	require.NoError(t, err)

	require.Equal(t, []string{"c5", "c3"}, []string{first[0].ID, first[1].ID})
	require.Len(t, rest, 1)
	require.Equal(t, "c1", rest[0].ID)
}

func Test_Requests_ListFiltersByService(t *testing.T) {
	repo := mysql.NewRequestRepository(testdb.Open(t))
	api := request("c1", "t1", "deploy")
	web := request("c2", "t1", "deploy")
	web.ServiceID, web.ServiceSlug = "s2", "web"
	require.NoError(t, repo.Create(ctx, api))
	require.NoError(t, repo.Create(ctx, web))

	found, err := repo.List(ctx, entities.Filter{Status: entities.StatusPending, ServiceID: "s2", Limit: 10})

	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "c2", found[0].ID)
}

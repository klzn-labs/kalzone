package submit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/submit"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	bob = actor.Actor{UserID: "u-bob", Login: "bob", Token: actor.Token{ID: "k1", Name: "claude-local"}}
)

type store struct {
	byID       map[string]entities.Request
	pendingErr error
	skew       time.Duration
}

func (s *store) Create(_ context.Context, r entities.Request) error {
	r.CreatedAt = r.CreatedAt.Round(time.Millisecond).Add(s.skew)
	s.byID[r.ID] = r
	return nil
}

func (s *store) PendingFor(_ context.Context, serviceID, resource, teamID, environment, subject string) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if r.ServiceID == serviceID && r.Resource == resource && r.TeamID == teamID && r.Environment == environment && r.Subject == subject && r.Pending() {
			found = append(found, r)
		}
	}
	return found, s.pendingErr
}

func (s *store) Transition(_ context.Context, r entities.Request, from entities.Status) error {
	if s.byID[r.ID].Status != from {
		return apperror.Conflict("changed")
	}
	s.byID[r.ID] = r
	return nil
}

type notes struct{ texts []string }

func (n *notes) Notify(text string) { n.texts = append(n.texts, text) }

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	store   *store
	notes   *notes
	events  *recorder
	usecase submit.SubmitUsecase
}

func newFixture() fixture {
	clock.WithTime(now)
	f := fixture{store: &store{byID: map[string]entities.Request{}}, notes: &notes{}, events: &recorder{}}
	f.usecase = submit.NewSubmitUsecase(config.Config{BaseURL: "http://platform.test/"}, f.store, f.notes, f.events)
	return f
}

func change(subject string) approval.Change {
	return approval.Change{
		Kind: approval.KindDeploy, Subject: subject, ServiceID: "s1", ServiceSlug: "api", TeamID: "t1",
		Environment: "prod", Summary: "deploy release #2 to prod", Proposal: map[string]string{"release_id": "r2"},
	}
}

func Test_Submit_StoresAPendingRequestAndSaysWhereToApproveIt(t *testing.T) {
	f := newFixture()

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	stored := f.store.byID[pending.ID]
	require.Equal(t, "change_request", pending.Kind)
	require.Equal(t, "http://platform.test/requests/"+pending.ID, pending.ApproveURL)
	require.Equal(t, "/api/v1/requests/"+pending.ID, pending.Poll)
	require.Equal(t, now.Add(24*time.Hour), pending.ExpiresAt)
	require.Equal(t, entities.StatusPending, stored.Status)
	require.Equal(t, "claude-local", stored.TokenName)
	require.JSONEq(t, `{"release_id":"r2"}`, string(stored.Payload))
	require.Equal(t, []string{"request.created"}, f.events.actions)
	require.Contains(t, f.notes.texts[0], "bob via claude-local asks to deploy release #2 to prod on api in prod")
	require.Contains(t, f.notes.texts[0], pending.ApproveURL)
}

func Test_Submit_SupersedesOlderPendingRequestsForTheSameSubject(t *testing.T) {
	f := newFixture()
	first, _ := f.usecase.Submit(ctx, bob, change("deploy"))
	other, _ := f.usecase.Submit(ctx, bob, change("secret.TOKEN"))
	clock.WithTime(now.Add(time.Minute))

	second, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	older := f.store.byID[first.ID]
	require.Equal(t, entities.StatusCancelled, older.Status)
	require.Equal(t, "superseded by "+second.ID, older.Reason)
	require.Equal(t, entities.StatusPending, f.store.byID[other.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[second.ID].Status)
	require.Equal(t, []string{"request.created", "request.created", "request.superseded", "request.created"}, f.events.actions)
}

func Test_Submit_LeavesANewerPendingRequestAlone(t *testing.T) {
	f := newFixture()
	newer := entities.Request{ID: "newer", ServiceID: "s1", Environment: "prod", Subject: "deploy", Status: entities.StatusPending, CreatedAt: now.Add(time.Second)}
	f.store.byID[newer.ID] = newer

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	require.Equal(t, entities.StatusPending, f.store.byID["newer"].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[pending.ID].Status)
	require.NotContains(t, f.events.actions, "request.superseded")
}

func Test_Submit_NeverSupersedesTheRequestItJustStored(t *testing.T) {
	f := newFixture()
	clock.WithTime(now.Add(400 * time.Microsecond))

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	require.Equal(t, entities.StatusPending, f.store.byID[pending.ID].Status)
	require.NotContains(t, f.events.actions, "request.superseded")
}

func Test_Submit_NeverSupersedesItselfWhateverTheStoredTimeSays(t *testing.T) {
	f := newFixture()
	f.store.skew = -time.Second

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	require.Equal(t, entities.StatusPending, f.store.byID[pending.ID].Status)
	require.NotContains(t, f.events.actions, "request.superseded")
}

func Test_Submit_ReturnsTheStoredRequestEvenWhenSupersedingFails(t *testing.T) {
	f := newFixture()
	f.store.pendingErr = errors.New("db went away")

	pending, err := f.usecase.Submit(ctx, bob, change("deploy"))

	require.NoError(t, err)
	require.Equal(t, entities.StatusPending, f.store.byID[pending.ID].Status)
	require.Equal(t, []string{"request.created"}, f.events.actions)
	require.Len(t, f.notes.texts, 1)
}

func Test_Submit_ClipsALongSummary(t *testing.T) {
	f := newFixture()
	long := change("config")
	long.Summary = "change " + strings.Repeat("env.KEY, ", 60)

	pending, err := f.usecase.Submit(ctx, bob, long)

	require.NoError(t, err)
	require.Len(t, []rune(f.store.byID[pending.ID].Summary), entities.SummaryLimit)
}

func databaseChange(subject string) approval.Change {
	return approval.Change{
		Kind: approval.KindDatabaseCreate, Subject: subject, Resource: "database shop", TeamID: "t1",
		Environment: "prod", Summary: "create database shop", Proposal: map[string]string{"name": "shop"},
	}
}

func Test_Submit_SupersedesByResourceSubject(t *testing.T) {
	f := newFixture()
	first, _ := f.usecase.Submit(ctx, bob, databaseChange("database.shop"))
	other, _ := f.usecase.Submit(ctx, bob, databaseChange("database.cart"))
	otherResource := databaseChange("database.shop")
	otherResource.Resource = "database other"
	sibling, _ := f.usecase.Submit(ctx, bob, otherResource)
	otherTeam := databaseChange("database.shop")
	otherTeam.TeamID = "t2"
	foreign, _ := f.usecase.Submit(ctx, bob, otherTeam)
	clock.WithTime(now.Add(time.Minute))

	second, err := f.usecase.Submit(ctx, bob, databaseChange("database.shop"))

	require.NoError(t, err)
	require.Equal(t, entities.StatusCancelled, f.store.byID[first.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[other.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[sibling.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[foreign.ID].Status)
	require.Equal(t, entities.StatusPending, f.store.byID[second.ID].Status)
	require.Equal(t, "database shop", f.store.byID[second.ID].Resource)
}

func Test_Notice_NamesTheResource(t *testing.T) {
	f := newFixture()

	_, err := f.usecase.Submit(ctx, bob, databaseChange("database.shop"))

	require.NoError(t, err)
	require.Contains(t, f.notes.texts[0], "bob via claude-local asks to create database shop in prod.")
}

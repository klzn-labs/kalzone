package crons_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_Create_UsesTheServiceTeamAndDefaults(t *testing.T) {
	f := newFixture(t)

	c, err := f.usecase.Create(ctx, ana, draft())

	require.NoError(t, err)
	require.Equal(t, "t-plateful", c.TeamID)
	require.Equal(t, "s-worker", c.ServiceID)
	require.Equal(t, time.Minute, c.Timeout)
	require.Nil(t, c.Payload)
	require.Equal(t, c, f.store.crons["prod/nightly-import"])
	require.Equal(t, []string{"cron.created"}, f.events.actions())
}

func Test_Create_KeepsAValidPayload(t *testing.T) {
	f := newFixture(t)
	d := draft()
	d.Payload, d.Timeout = []byte(` {"full":true} `), 5*time.Minute

	c, err := f.usecase.Create(ctx, ana, d)

	require.NoError(t, err)
	require.Equal(t, `{"full":true}`, string(c.Payload))
	require.Equal(t, 5*time.Minute, c.Timeout)
}

func Test_Create_RefusesBadDrafts(t *testing.T) {
	cases := map[string]func(*crons.Draft){
		"bad name":       func(d *crons.Draft) { d.Name = "X" },
		"bad schedule":   func(d *crons.Draft) { d.Schedule = "@every 30s" },
		"bad path":       func(d *crons.Draft) { d.Path = "hooks" },
		"bad payload":    func(d *crons.Draft) { d.Payload = []byte("{") },
		"short timeout":  func(d *crons.Draft) { d.Timeout = time.Millisecond },
		"no environment": func(d *crons.Draft) { d.Environment = "qa" },
		"no service":     func(d *crons.Draft) { d.Service = "ghost" },
	}
	for name, change := range cases {
		f := newFixture(t)
		d := draft()
		change(&d)

		_, err := f.usecase.Create(ctx, ana, d)

		require.Error(t, err, name)
		require.Empty(t, f.store.crons, name)
	}
}

func Test_Create_RefusesAServiceBeingDeleted(t *testing.T) {
	f := newFixture(t)
	d := draft()
	d.Service = "leaving"

	_, err := f.usecase.Create(ctx, ana, d)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_Create_ATakenNameIsAConflictEvenBeforeApproval(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, direct := f.usecase.Create(ctx, ana, draft())
	_, gated := f.as(authz.NeedsApproval).Create(ctx, ana, draft())

	require.ErrorIs(t, direct, apperror.Conflict(""))
	require.ErrorIs(t, gated, apperror.Conflict(""))
	require.Empty(t, f.requests.changes)
}

func Test_Create_RefusedByPolicy(t *testing.T) {
	f := newFixture(t)

	_, err := f.denied().Create(ctx, ana, draft())

	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.Empty(t, f.store.crons)
}

func Test_Update_ChangesOnlyTheGivenFields(t *testing.T) {
	f := newFixture(t)
	created, _ := f.usecase.Create(ctx, ana, draft())

	updated, err := f.usecase.Update(ctx, ana, "prod", "nightly-import", crons.Change{Schedule: ptr("@hourly"), Payload: ptr([]byte(`[1]`))})

	require.NoError(t, err)
	require.Equal(t, "@hourly", updated.Schedule)
	require.Equal(t, created.Path, updated.Path)
	require.Equal(t, `[1]`, string(updated.Payload))
	require.Equal(t, updated, f.store.crons["prod/nightly-import"])
	require.Equal(t, []string{"cron.created", "cron.updated"}, f.events.actions())
}

func Test_Update_RefusesAnInvalidChange(t *testing.T) {
	f := newFixture(t)
	created, _ := f.usecase.Create(ctx, ana, draft())

	_, err := f.usecase.Update(ctx, ana, "prod", "nightly-import", crons.Change{Timeout: ptr(time.Hour)})

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Equal(t, created, f.store.crons["prod/nightly-import"])
}

func Test_SetPaused_PausesAndResumes(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	paused, err := f.usecase.SetPaused(ctx, ana, "prod", "nightly-import", true)
	require.NoError(t, err)
	resumed, _ := f.usecase.SetPaused(ctx, ana, "prod", "nightly-import", false)

	require.True(t, paused.Paused)
	require.False(t, resumed.Paused)
	require.False(t, f.store.crons["prod/nightly-import"].Paused)
	require.Equal(t, []string{"cron.created", "cron.paused", "cron.resumed"}, f.events.actions())
}

func Test_Run_AddsACommand(t *testing.T) {
	f := newFixture(t)
	created, _ := f.usecase.Create(ctx, ana, draft())

	command, err := f.usecase.Run(ctx, ana, "prod", "nightly-import")

	require.NoError(t, err)
	require.Equal(t, []entities.Command{command}, f.store.commands)
	require.Equal(t, created.ID, command.CronID)
	require.Equal(t, entities.CommandRun, command.Kind)
	require.Equal(t, command.ID, f.events.events[1].Details["run"])
}

func Test_PauseAndRunStayRefusedWithoutBypass(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, paused := f.as(authz.NeedsApproval).SetPaused(ctx, ana, "prod", "nightly-import", true)
	_, ran := f.as(authz.NeedsApproval).Run(ctx, ana, "prod", "nightly-import")

	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(paused))
	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(ran))
	require.Empty(t, f.store.commands)
	require.Empty(t, f.requests.changes)
}

func Test_Delete_RemovesTheCron(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	err := f.usecase.Delete(ctx, ana, "prod", "nightly-import")

	require.NoError(t, err)
	require.Empty(t, f.store.crons)
	require.Equal(t, []string{"cron.created", "cron.deleted"}, f.events.actions())
}

func Test_DeleteForService_RemovesOnlyThatServicesCrons(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())
	dev := draft()
	dev.Environment = "dev"
	_, _ = f.usecase.Create(ctx, ana, dev)
	f.store.crons["prod/other"] = entities.Cron{ID: "other", Name: "other", Environment: "prod", ServiceID: "s-other"}

	err := f.usecase.DeleteForService(ctx, actor.Actor{Login: "decommission"}, "s-worker")

	require.NoError(t, err)
	require.Len(t, f.store.crons, 1)
	require.Contains(t, f.store.crons, "prod/other")
	require.Equal(t, "service deleted", f.events.events[len(f.events.events)-1].Details["reason"])
}

func Test_ListFor_HidesOtherTeams(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())
	f.store.crons["prod/secret"] = entities.Cron{ID: "secret", Name: "secret", Environment: "prod", TeamID: "t-other"}

	list, err := f.usecase.ListFor(ctx, ana, "", "")
	_, hidden := f.usecase.GetFor(ctx, ana, "prod", "secret")

	require.NoError(t, err)
	require.Len(t, list, 1)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}

func Test_RunsFor_AndLatestRun(t *testing.T) {
	f := newFixture(t)
	created, _ := f.usecase.Create(ctx, ana, draft())
	f.store.runs = []entities.Run{{ID: "r2", CronID: created.ID}, {ID: "r1", CronID: created.ID}, {ID: "x", CronID: "other"}}

	runs, err := f.usecase.RunsFor(ctx, ana, "prod", "nightly-import")
	latest, found, _ := f.usecase.LatestRun(ctx, created)
	_, none, _ := f.usecase.LatestRun(ctx, entities.Cron{ID: "empty"})

	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.True(t, found)
	require.Equal(t, "r2", latest.ID)
	require.False(t, none)
}

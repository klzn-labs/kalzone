package databases_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func platefulDraft() databases.AdoptDraft {
	return databases.AdoptDraft{
		Schema: "plateful", Name: "plateful", Environment: "prod", Team: "plateful",
		MaxConnections: 30, Services: []string{"api", "worker", "importer"},
	}
}

func adoptFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.provisioner.existing["plateful"] = true
	return f
}

func (f fixture) adopt(t *testing.T, d databases.AdoptDraft) entities.Database {
	t.Helper()
	db, err := f.open().Adopt(ctx, ana, d)
	require.NoError(t, err)
	return db
}

func maintainer() policy {
	return policy{denied: map[authz.Action]string{authz.AdoptDatabase: ""}}
}

func Test_Adopt_NeedsAnAdmin(t *testing.T) {
	f := adoptFixture(t)

	_, err := f.with(maintainer()).Adopt(ctx, ana, platefulDraft())

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Zero(t, f.provisioner.reads)
	require.Zero(t, f.provisioner.writes)
	require.Empty(t, f.store.databases)
}

func Test_Adopt_RecordsAndAttaches(t *testing.T) {
	f := adoptFixture(t)

	db := f.adopt(t, platefulDraft())

	require.True(t, db.Adopted)
	require.Equal(t, "plateful", db.Schema)
	require.Equal(t, "kz_plateful", db.Username)
	require.Equal(t, 30, db.MaxConnections)
	require.Equal(t, "t-plateful", db.TeamID)
	require.Equal(t, db, f.store.databases[db.ID])
	attachments, _ := f.store.Attachments(ctx, db.ID)
	require.Len(t, attachments, 3)
	for _, a := range attachments {
		require.True(t, a.Active())
		require.Equal(t, "prod", a.Environment)
	}
	requireAdoptedEvent(t, f, requireAdoptCall(t, f))
}

func requireAdoptCall(t *testing.T, f fixture) string {
	t.Helper()
	require.Len(t, f.provisioner.adopted, 1)
	got := f.provisioner.adopted[0]
	require.Equal(t, "plateful", got.schema)
	require.Equal(t, "kz_plateful", got.user)
	require.Equal(t, 30, got.maxConnections)
	require.Len(t, got.password, 64)
	require.Empty(t, f.provisioner.created)
	return got.password
}

func requireAdoptedEvent(t *testing.T, f fixture, password string) {
	t.Helper()
	require.Equal(t, []string{"database.adopted"}, f.events.actions())
	details := f.events.events[0].Details
	require.Equal(t, "plateful", details["schema"])
	require.Equal(t, "api, worker, importer", details["services"])
	require.Equal(t, 30, details["max_connections"])
	for key, value := range details {
		require.NotContains(t, fmt.Sprint(value), password, key)
	}
	require.NotContains(t, fmt.Sprint(f.events.events[0]), password)
}

type adoptRefusal struct {
	name    string
	prepare func(f fixture, d *databases.AdoptDraft)
	code    apperror.Code
	hint    string
}

var adoptRefusals = []adoptRefusal{
	{"a reserved schema", func(f fixture, d *databases.AdoptDraft) {
		d.Schema = "platform"
		f.provisioner.existing["platform"] = true
	}, apperror.CodeInvalidInput, ""},
	{"a schema another record uses", func(f fixture, _ *databases.AdoptDraft) {
		f.store.databases["x"] = entities.Database{ID: "x", Name: "legacy", Environment: "dev", Schema: "plateful"}
	}, apperror.CodeConflict, ""},
	{"a schema missing in MySQL", func(_ fixture, d *databases.AdoptDraft) { d.Schema = "nosuch" }, apperror.CodeNotFound, "check the schema name in MySQL"},
	{"an invalid schema", func(_ fixture, d *databases.AdoptDraft) { d.Schema = "Plate-ful" }, apperror.CodeInvalidInput, ""},
	{"an invalid name", func(_ fixture, d *databases.AdoptDraft) { d.Name = "Bad Name" }, apperror.CodeInvalidInput, ""},
	{"a name taken in the environment", func(f fixture, _ *databases.AdoptDraft) {
		f.store.databases["x"] = entities.Database{ID: "x", Name: "plateful", Environment: "prod", Schema: "elsewhere"}
	}, apperror.CodeConflict, ""},
	{"an unknown service", func(_ fixture, d *databases.AdoptDraft) { d.Services = []string{"api", "ghost"} }, apperror.CodeNotFound, ""},
	{"a service attached elsewhere", func(f fixture, _ *databases.AdoptDraft) {
		f.store.databases["x"] = entities.Database{ID: "x", Name: "legacy", Environment: "prod", Schema: "legacy"}
		f.store.attachments = append(f.store.attachments, entities.Attachment{DatabaseID: "x", ServiceID: "s-worker", Environment: "prod"})
	}, apperror.CodeConflict, "detach legacy from worker first"},
	{"a service listed twice", func(_ fixture, d *databases.AdoptDraft) { d.Services = []string{"api", "api"} }, apperror.CodeInvalidInput, ""},
	{"too many connections", func(_ fixture, d *databases.AdoptDraft) { d.MaxConnections = 31 }, apperror.CodeInvalidInput, ""},
	{"an unknown environment", func(_ fixture, d *databases.AdoptDraft) { d.Environment = "qa" }, apperror.CodeNotFound, ""},
}

func Test_Adopt_Refuses(t *testing.T) {
	for _, c := range adoptRefusals {
		t.Run(c.name, func(t *testing.T) {
			f := adoptFixture(t)
			d := platefulDraft()
			c.prepare(f, &d)
			before, attachments := len(f.store.databases), len(f.store.attachments)

			_, err := f.open().Adopt(ctx, ana, d)

			require.Equal(t, c.code, apperror.CodeOf(err), "%v", err)
			if c.hint != "" {
				require.Equal(t, c.hint, hintOf(t, err))
			}
			require.Zero(t, f.provisioner.writes)
			require.Len(t, f.store.databases, before)
			require.Len(t, f.store.attachments, attachments)
			require.Empty(t, f.events.events)
		})
	}
}

func Test_Adopt_KeepsNoRecordWhenTheProvisionerRefuses(t *testing.T) {
	f := adoptFixture(t)
	refusal := apperror.Conflict("this schema has not been prepared for adoption")
	f.provisioner.failAdopt = refusal

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.ErrorIs(t, err, refusal)
	require.Empty(t, f.store.databases)
	require.Empty(t, f.store.attachments)
	require.Empty(t, f.provisioner.released)
	require.Empty(t, f.events.events)
}

func Test_Adopt_ReportsARecordThatCannotBeRemovedAfterARefusal(t *testing.T) {
	f := adoptFixture(t)
	refusal := apperror.Conflict("this schema has not been prepared for adoption")
	f.provisioner.failAdopt, f.store.failDelete = refusal, errBroken

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.ErrorIs(t, err, refusal)
	require.ErrorIs(t, err, errBroken)
	require.Empty(t, f.events.events)
}

func Test_Adopt_TouchesNoUserWhenTheRecordIsNotStored(t *testing.T) {
	f := adoptFixture(t)
	f.store.failCreate = errBroken

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.ErrorIs(t, err, errBroken)
	require.Zero(t, f.provisioner.writes)
	require.Empty(t, f.store.databases)
	require.Empty(t, f.store.attachments)
}

func Test_Adopt_ForgetsARecordStoredDespiteAnError(t *testing.T) {
	f := adoptFixture(t)
	f.store.failCreate, f.store.savedAnyway = errBroken, true

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.ErrorIs(t, err, errBroken)
	require.Zero(t, f.provisioner.writes)
	require.Empty(t, f.store.databases)
}

func Test_Adopt_DefaultsMaxConnections(t *testing.T) {
	f := adoptFixture(t)
	d := platefulDraft()
	d.MaxConnections = 0

	db := f.adopt(t, d)

	require.Equal(t, 10, db.MaxConnections)
	require.Equal(t, 10, f.provisioner.adopted[0].maxConnections)
}

func Test_Adopt_GivesTheAttachedServicesTheNewCredentials(t *testing.T) {
	f := adoptFixture(t)
	db := f.adopt(t, platefulDraft())

	env, secrets, err := f.open().Access(ctx, "s-importer", "prod")

	require.NoError(t, err)
	require.Equal(t, "plateful", env["PLATFORM_DB_NAME"])
	require.Equal(t, "kz_plateful", secrets["PLATFORM_DB_USER"])
	require.Equal(t, f.provisioner.adopted[0].password, secrets["PLATFORM_DB_PASSWORD"])
	require.NotContains(t, string(db.PasswordCiphertext), secrets["PLATFORM_DB_PASSWORD"])
}

func Test_Adopt_LeavesThePasswordOfARivalThatStoredTheSchemaFirst(t *testing.T) {
	f := adoptFixture(t)
	f.store.failCreate = apperror.Conflict("duplicate schema")
	f.store.rival = &entities.Database{ID: "rival", Name: "plateful", Environment: "prod", Schema: "plateful", Adopted: true}

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Zero(t, f.provisioner.writes)
	require.Empty(t, f.provisioner.adopted)
	require.Equal(t, []string{"rival"}, slices.Collect(maps.Keys(f.store.databases)))
	require.Empty(t, f.events.events)
}

func Test_Adopt_AuditsTheServicesAttachedBeforeAFailure(t *testing.T) {
	f := adoptFixture(t)
	f.store.failAttachOn = "s-importer"

	_, err := f.open().Adopt(ctx, ana, platefulDraft())

	require.ErrorIs(t, err, errBroken)
	require.Equal(t, []string{"database.adopted"}, f.events.actions())
	require.Equal(t, "api, worker", f.events.events[0].Details["services"])
	require.Len(t, f.store.databases, 1)
}

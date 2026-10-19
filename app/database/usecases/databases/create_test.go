package databases_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

func Test_Create_ProvisionsAndSealsThePassword(t *testing.T) {
	f := newFixture(t)

	db := f.created(t, shopDraft())

	require.Len(t, f.provisioner.created, 1)
	got := f.provisioner.created[0]
	require.Equal(t, "shop_dev", got.schema)
	require.Equal(t, "kz_shop_dev", got.user)
	require.Equal(t, 10, got.maxConnections)
	require.Len(t, got.password, 64)
	stored := f.store.databases[db.ID]
	require.Equal(t, "t-plateful", stored.TeamID)
	require.Equal(t, "k1", stored.PasswordKeyID)
	require.NotContains(t, string(stored.PasswordCiphertext), got.password)
	require.Equal(t, []string{"database.created"}, f.events.actions())
	require.NotContains(t, fmt.Sprint(f.events.events[0]), got.password)
	require.Equal(t, events.TargetDatabase, f.events.events[0].TargetKind)
}

func Test_Create_ProdUsesTheBareSchema(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment, d.MaxConnections = "prod", 5

	f.created(t, d)

	require.Equal(t, "shop", f.provisioner.created[0].schema)
	require.Equal(t, 5, f.provisioner.created[0].maxConnections)
}

func Test_Create_RefusesExistingSchema(t *testing.T) {
	f := newFixture(t)
	f.provisioner.existing["shop_dev"] = true

	_, err := f.open().Create(ctx, ana, shopDraft())

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Empty(t, f.provisioner.created)
	require.Empty(t, f.provisioner.dropped)
	require.Empty(t, f.store.databases)
}

func Test_Create_RefusesSchemaOfAnotherRow(t *testing.T) {
	f := newFixture(t)
	f.created(t, databases.Draft{Name: "ab", Environment: "dev", Team: "plateful"})

	_, err := f.open().Create(ctx, ana, databases.Draft{Name: "ab-dev", Environment: "prod", Team: "plateful"})

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Len(t, f.provisioner.created, 1)
}

func Test_Create_RefusesReservedSchema(t *testing.T) {
	f := newFixture(t)

	_, err := f.open().Create(ctx, ana, databases.Draft{Name: "platform", Environment: "prod", Team: "plateful"})

	require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err))
	require.Zero(t, f.provisioner.reads)
	require.Zero(t, f.provisioner.writes)
}

func Test_Create_RefusesBadInput(t *testing.T) {
	f := newFixture(t)
	tooMany := shopDraft()
	tooMany.MaxConnections = 31
	noEnv := shopDraft()
	noEnv.Environment = "qa"

	for _, d := range []databases.Draft{{Name: "S", Environment: "dev", Team: "plateful"}, tooMany, noEnv} {
		_, err := f.open().Create(ctx, ana, d)
		require.Error(t, err)
	}
	require.Zero(t, f.provisioner.reads)
	require.Zero(t, f.provisioner.writes)
}

func Test_Create_ForbiddenTouchesNothing(t *testing.T) {
	f := newFixture(t)

	_, err := f.with(policy{denied: map[authz.Action]string{authz.EditDatabase: ""}}).Create(ctx, ana, shopDraft())

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Zero(t, f.provisioner.reads)
	require.Zero(t, f.provisioner.writes)
	require.Empty(t, f.requests.changes)
}

func Test_Create_ProtectedEnvSubmitsWithoutSecrets(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment = "prod"

	_, err := f.as(authz.NeedsApproval).Create(ctx, ana, d)

	var pending *approval.Pending
	require.ErrorAs(t, err, &pending)
	require.Zero(t, f.provisioner.writes)
	require.Len(t, f.requests.changes, 1)
	change := f.requests.changes[0]
	require.Equal(t, approval.KindDatabaseCreate, change.Kind)
	require.Equal(t, "database.shop", change.Subject)
	require.Equal(t, "database shop", change.Resource)
	require.Equal(t, "t-plateful", change.TeamID)
	payload, err := json.Marshal(change.Proposal)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "password")
	require.Empty(t, f.store.databases)
}

func Test_Create_DropsWhenTheRowFails(t *testing.T) {
	f := newFixture(t)
	f.store.failCreate = errBroken

	_, err := f.open().Create(ctx, ana, shopDraft())

	require.ErrorIs(t, err, errBroken)
	require.Equal(t, []string{"shop_dev/kz_shop_dev"}, f.provisioner.dropped)
	require.Empty(t, f.events.events)
}

func Test_Create_KeepsSchemaWhenTheRowWasSavedAnyway(t *testing.T) {
	f := newFixture(t)
	f.store.failCreate, f.store.savedAnyway = errBroken, true

	_, err := f.open().Create(ctx, ana, shopDraft())

	require.ErrorIs(t, err, errBroken)
	require.Empty(t, f.provisioner.dropped)
	require.Len(t, f.store.databases, 1)
}

func Test_Create_ProtectedEnvRefusesExistingSchemaBeforeFiling(t *testing.T) {
	f := newFixture(t)
	f.provisioner.existing["shop"] = true
	d := shopDraft()
	d.Environment = "prod"

	_, err := f.as(authz.NeedsApproval).Create(ctx, ana, d)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Empty(t, f.requests.changes)
	require.Zero(t, f.provisioner.writes)
}

func Test_Create_NeverDropsAfterAFailedProvision(t *testing.T) {
	f := newFixture(t)
	f.provisioner.failCreate = errBroken

	_, err := f.open().Create(ctx, ana, shopDraft())

	require.ErrorIs(t, err, errBroken)
	require.Empty(t, f.provisioner.dropped)
	require.Empty(t, f.store.databases)
}

func Test_Create_RefusesTakenName(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	_, err := f.open().Create(ctx, ana, shopDraft())

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Len(t, f.provisioner.created, 1)
	require.Empty(t, f.provisioner.dropped)
}

func Test_Create_KeepsTheRowEntitiesConsistent(t *testing.T) {
	f := newFixture(t)

	db := f.created(t, shopDraft())

	require.Equal(t, entities.UsernamePrefix+db.Schema, db.Username)
	require.False(t, db.Adopted)
}

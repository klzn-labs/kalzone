package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var at = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func database(id, env, name string) entities.Database {
	schema := entities.SchemaFor(name, env, false)
	return entities.Database{
		ID: id, Name: name, Environment: env, TeamID: "team", Schema: schema, Username: "kz_" + schema,
		PasswordCiphertext: []byte("cipher"), PasswordNonce: []byte("nonce"), PasswordKeyID: "k1", MaxConnections: 10,
		CreatedAt: at, UpdatedAt: at,
	}
}

func attachment(databaseID, serviceID, env string) entities.Attachment {
	return entities.Attachment{DatabaseID: databaseID, ServiceID: serviceID, Environment: env, AttachedAt: at}
}

func Test_Create_GetRoundTrips(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	d := database("d1", "dev", "orders")
	require.NoError(t, repo.Create(ctx, d))

	got, err := repo.Get(ctx, "dev", "orders")

	require.NoError(t, err)
	require.Equal(t, d, got)
}

func Test_Get_MissingIsNotFound(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))

	_, err := repo.Get(ctx, "dev", "nope")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_DuplicateIsConflict(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))

	err := repo.Create(ctx, database("d2", "dev", "orders"))

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_List_FiltersByEnvironmentAndTeam(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	other := database("d2", "prod", "billing")
	other.TeamID = "other"
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Create(ctx, other))

	all, err := repo.List(ctx, "", "")
	require.NoError(t, err)
	byEnv, _ := repo.List(ctx, "prod", "")
	byTeam, _ := repo.List(ctx, "", "team")

	require.Len(t, all, 2)
	require.Len(t, byEnv, 1)
	require.Equal(t, "d1", byTeam[0].ID)
}

func Test_ActiveFor_IgnoresDetached(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "svc", "dev")))

	got, ok, err := repo.ActiveFor(ctx, "svc", "dev")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "d1", got.ID)

	require.NoError(t, repo.Detach(ctx, "d1", "svc", at))
	_, ok, err = repo.ActiveFor(ctx, "svc", "dev")
	require.NoError(t, err)
	require.False(t, ok)
}

func Test_Attachments_KeepsDrainingRows(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "a", "dev")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "b", "dev")))
	require.NoError(t, repo.Detach(ctx, "d1", "b", at))

	got, err := repo.Attachments(ctx, "d1")

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[0].Active())
	require.False(t, got[1].Active())
}

func Test_ForgetDrained_RemovesOnlyDetachedRow(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "a", "dev")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "b", "dev")))
	require.NoError(t, repo.Detach(ctx, "d1", "b", at))

	require.NoError(t, repo.ForgetDrained(ctx, "d1", "a"))
	require.NoError(t, repo.ForgetDrained(ctx, "d1", "b"))

	got, _ := repo.Attachments(ctx, "d1")
	require.Len(t, got, 1)
	require.Equal(t, "a", got[0].ServiceID)
}

func Test_Delete_RemovesAttachments(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "a", "dev")))

	require.NoError(t, repo.Delete(ctx, "d1"))

	_, err := repo.GetByID(ctx, "d1")
	require.ErrorIs(t, err, apperror.NotFound(""))
	got, _ := repo.Attachments(ctx, "d1")
	require.Empty(t, got)
}

func Test_DetachService_StampsEveryActiveRow(t *testing.T) {
	repo := mysql.NewDatabaseRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, database("d1", "dev", "orders")))
	require.NoError(t, repo.Create(ctx, database("d2", "prod", "orders")))
	require.NoError(t, repo.Attach(ctx, attachment("d1", "svc", "dev")))
	require.NoError(t, repo.Attach(ctx, attachment("d2", "svc", "prod")))

	require.NoError(t, repo.DetachService(ctx, "svc", at))

	for _, id := range []string{"d1", "d2"} {
		got, _ := repo.Attachments(ctx, id)
		require.False(t, got[0].Active(), id)
	}
}

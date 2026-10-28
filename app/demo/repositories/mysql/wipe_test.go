package mysql_test

import (
	"context"
	"testing"
	"time"

	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	auditMysql "github.com/klzn-labs/kalzone/app/audit/repositories/mysql"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	catalogMysql "github.com/klzn-labs/kalzone/app/catalog/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/demo/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func Test_Wipe_RemovesTheDemoTeamAndItsPersonasAndLeavesOtherTeamsAlone(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO teams (id, slug, name, is_demo, created_at) VALUES ('t-demo', 'demo', 'Demo', TRUE, ?), ('t1', 'places', 'Places', FALSE, ?)", at, at).Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, login, name, kind, created_at, last_login_at) VALUES ('u-ana', 'ana.demo', 'Ana', 'persona', ?, ?), ('u-bob', 'bob', 'Bob', 'person', ?, ?)", at, at, at, at).Error)
	require.NoError(t, db.Exec("INSERT INTO memberships (team_id, user_id, role, created_at) VALUES ('t-demo', 'u-ana', 'maintainer', ?), ('t1', 'u-bob', 'member', ?)", at, at).Error)
	services := catalogMysql.NewServiceRepository(db)
	require.NoError(t, services.Create(ctx, catalog.Service{ID: "s-demo", Slug: "demo-shop", Name: "Shop", OwnerTeamID: "t-demo", CreatedAt: at}))
	require.NoError(t, services.Create(ctx, catalog.Service{ID: "s1", Slug: "places-api", Name: "Places", OwnerTeamID: "t1", CreatedAt: at}))
	events := auditMysql.NewEventRepository(db)
	require.NoError(t, events.Insert(ctx, audit.Event{ID: "e1", At: at, Action: "x", TargetKind: "service", TeamID: "t-demo"}))
	require.NoError(t, events.Insert(ctx, audit.Event{ID: "e2", At: at, Action: "x", TargetKind: "service", TeamID: "t1"}))

	require.NoError(t, mysql.NewWipeRepository(db).Wipe(ctx, "t-demo"))

	require.Equal(t, []string{"t1"}, column(t, db.Raw("SELECT id FROM teams")))
	require.Equal(t, []string{"u-bob"}, column(t, db.Raw("SELECT id FROM users")))
	require.Equal(t, []string{"t1"}, column(t, db.Raw("SELECT team_id FROM memberships")))
	require.Equal(t, []string{"s1"}, column(t, db.Raw("SELECT id FROM services")))
	require.Equal(t, []string{"e2"}, column(t, db.Raw("SELECT id FROM audit_events")))
}

func column(t *testing.T, query *gorm.DB) []string {
	t.Helper()
	var values []string
	require.NoError(t, query.Scan(&values).Error)
	return values
}

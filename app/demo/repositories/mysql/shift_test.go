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
)

func Test_Shift_MovesEveryDemoTimestampAndLeavesOtherTeamsAlone(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO teams (id, slug, name, is_demo, created_at) VALUES ('t-demo', 'demo', 'Demo', TRUE, ?), ('t1', 'places', 'Places', FALSE, ?)", at, at).Error)
	require.NoError(t, catalogMysql.NewServiceRepository(db).Create(ctx, catalog.Service{ID: "s-demo", Slug: "demo-shop", Name: "Shop", OwnerTeamID: "t-demo", CreatedAt: at}))
	events := auditMysql.NewEventRepository(db)
	require.NoError(t, events.Insert(ctx, audit.Event{ID: "e1", At: at, Action: "x", TargetKind: "service", TeamID: "t-demo"}))
	require.NoError(t, events.Insert(ctx, audit.Event{ID: "e2", At: at, Action: "x", TargetKind: "service", TeamID: "t1"}))

	require.NoError(t, mysql.NewShiftRepository(db).Shift(ctx, "t-demo", 36*time.Hour))

	var demoTeam, otherTeam, service, demoEvent, otherEvent time.Time
	db.Raw("SELECT created_at FROM teams WHERE id = 't-demo'").Scan(&demoTeam)
	db.Raw("SELECT created_at FROM teams WHERE id = 't1'").Scan(&otherTeam)
	db.Raw("SELECT created_at FROM services WHERE id = 's-demo'").Scan(&service)
	db.Raw("SELECT at FROM audit_events WHERE id = 'e1'").Scan(&demoEvent)
	db.Raw("SELECT at FROM audit_events WHERE id = 'e2'").Scan(&otherEvent)
	require.Equal(t, at.Add(36*time.Hour), demoTeam.UTC())
	require.Equal(t, at.Add(36*time.Hour), service.UTC())
	require.Equal(t, at.Add(36*time.Hour), demoEvent.UTC())
	require.Equal(t, at, otherTeam.UTC())
	require.Equal(t, at, otherEvent.UTC())
}

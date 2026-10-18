package logs_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/logs"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	ana = actor.Actor{UserID: "ana", Login: "ana"}
	now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
)

type services struct{}

func (services) FindService(_ context.Context, slug string) (entities.LogSource, error) {
	if slug != "api" {
		return entities.LogSource{}, apperror.NotFound("service not found")
	}
	return entities.LogSource{ID: "s1", Slug: "api", OwnerTeamID: "t1"}, nil
}

type policy struct {
	err      error
	resource authz.Resource
	action   authz.Action
}

func (p *policy) Can(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) error {
	p.action, p.resource = action, r
	return p.err
}

type loki struct {
	query      string
	start, end time.Time
	limit      int
}

func (l *loki) LogLines(_ context.Context, uid, query string, start, end time.Time, limit int) ([]entities.LogLine, error) {
	l.query, l.start, l.end, l.limit = uid+" "+query, start, end, limit
	return []entities.LogLine{{Line: "hello"}}, nil
}

func setup(p *policy) (*loki, logs.LogsUsecase) {
	clock.Clock = func() time.Time { return now }
	l := &loki{}
	return l, logs.NewLogsUsecase(services{}, p, l, "loki")
}

func read(t *testing.T, u logs.LogsUsecase, env, since, limit, contains, colour string) ([]entities.LogLine, error) {
	t.Helper()
	q, err := logs.ParseQuery(since, limit, contains, colour)
	if err != nil {
		return nil, err
	}
	return u.Read(ctx, ana, "api", env, q)
}

func Test_Read_QueriesBothColoursOfTheWorkload(t *testing.T) {
	l, u := setup(&policy{})
	t.Cleanup(func() { clock.Clock = nil })

	lines, err := read(t, u, "dev", "", "", "", "")

	require.NoError(t, err)
	require.Equal(t, "hello", lines[0].Line)
	require.Equal(t, `loki {container=~"api-dev-(blue|green)"}`, l.query)
	require.Equal(t, now.Add(-15*time.Minute), l.start)
	require.Equal(t, now, l.end)
	require.Equal(t, 100, l.limit)
}

func Test_Read_FiltersByColourAndQuotedText(t *testing.T) {
	l, u := setup(&policy{})
	t.Cleanup(func() { clock.Clock = nil })

	_, err := read(t, u, "dev", "2h", "500", `say "hi" \o/`, "green")

	require.NoError(t, err)
	require.Equal(t, `loki {container=~"api-dev-green"} |= "say \"hi\" \\o/"`, l.query)
	require.Equal(t, 500, l.limit)
	require.Equal(t, now.Add(-2*time.Hour), l.start)
}

func Test_Read_EscapesTheEnvironmentInTheSelector(t *testing.T) {
	l, u := setup(&policy{})
	t.Cleanup(func() { clock.Clock = nil })

	_, err := read(t, u, `dev"}|x`, "", "", "", "")

	require.NoError(t, err)
	require.Equal(t, `loki {container=~"api-dev\"\\}\\|x-(blue|green)"}`, l.query)
}

func Test_Read_AsksForTheLogsPermissionOnTheOwningTeam(t *testing.T) {
	p := &policy{err: apperror.Forbidden("no")}
	_, u := setup(p)
	t.Cleanup(func() { clock.Clock = nil })

	_, err := read(t, u, "dev", "", "", "", "")

	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.Equal(t, authz.ReadLogs, p.action)
	require.Equal(t, authz.Resource{TeamID: "t1", Environment: "dev"}, p.resource)
}

func Test_ParseQuery_RefusesWrongValues(t *testing.T) {
	for name, q := range map[string][4]string{
		"since long":  {"25h", "", "", ""},
		"since word":  {"soon", "", "", ""},
		"since zero":  {"0s", "", "", ""},
		"limit big":   {"", "1001", "", ""},
		"limit word":  {"", "many", "", ""},
		"colour pink": {"", "", "", "pink"},
	} {
		_, err := logs.ParseQuery(q[0], q[1], q[2], q[3])
		require.ErrorIs(t, err, apperror.Invalid(""), name)
	}
}

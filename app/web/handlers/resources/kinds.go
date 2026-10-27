package resources

import (
	"context"

	"github.com/klzn-labs/kalzone/app/web/handlers/crons"
	"github.com/klzn-labs/kalzone/app/web/handlers/databases"
	"github.com/klzn-labs/kalzone/app/web/handlers/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const (
	queueKind    = "queue"
	databaseKind = "database"
	cronKind     = "cron"
)

type kindInfo struct {
	Kind    string
	Title   string
	Noun    string
	Nav     string
	Href    string
	NewHref string
	Lead    string
}

var kinds = map[string]kindInfo{
	queueKind: {Kind: queueKind, Title: "Queues", Noun: "queue", Nav: "queues", Href: "/resources/queues", NewHref: "/resources/new/queue",
		Lead: "One consumer service receives each message once, with retries and a rate limit. Publishers send with the token every deployed service gets."},
	databaseKind: {Kind: databaseKind, Title: "Databases", Noun: "database", Nav: "databases", Href: "/resources/databases", NewHref: "/resources/new/database",
		Lead: "MySQL 8 databases on the shared server. Attach one to a service and its credentials arrive on the next deploy."},
	cronKind: {Kind: cronKind, Title: "Crons", Noun: "cron", Nav: "crons", Href: "/resources/crons",
		Lead: "Kalzone calls a path on one of your services on a schedule and keeps a record of every run."},
}

type Section struct {
	kindInfo
	Count     int
	Queues    []queues.Row
	Databases []databases.Row
	Crons     []crons.Row
}

func (h ResourcesHandler) section(ctx context.Context, a actor.Actor, info kindInfo, environment string) (Section, error) {
	s := Section{kindInfo: info}
	var err error
	switch info.Kind {
	case queueKind:
		s.Queues, err = h.queues.Rows(ctx, a, environment)
		s.Count = len(s.Queues)
	case databaseKind:
		s.Databases, err = h.databases.Rows(ctx, a, environment)
		s.Count = len(s.Databases)
	default:
		s.Crons, err = h.crons.Rows(ctx, a, environment)
		s.Count = len(s.Crons)
	}
	return s, err
}

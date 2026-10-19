package databases

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Row struct {
	Database entities.Database
	Team     string
	Attached []string
}

type Attachment struct {
	Service  string
	Since    time.Time
	State    string
	Detached bool
}

type detailView struct {
	Database    entities.Database
	Team        string
	Address     string
	Attachments []Attachment
	Attachable  []string
	Blocker     string
	CanEdit     bool
}

type newView struct {
	Teams          []team.Team
	MaxConnections int
}

func (h DatabasesHandler) Rows(ctx context.Context, a actor.Actor, environment string) ([]Row, error) {
	list, err := h.databases.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	names, err := h.teamNames(ctx, list)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(list))
	for _, db := range list {
		views, err := h.databases.AttachmentsFor(ctx, a, db.Environment, db.Name)
		if err != nil {
			return nil, err
		}
		rows = append(rows, Row{Database: db, Team: names[db.TeamID], Attached: slugsOf(views, activeOnly)})
	}
	return rows, nil
}

func (h DatabasesHandler) teamNames(ctx context.Context, list []entities.Database) (map[string]string, error) {
	ids := make([]string, 0, len(list))
	for _, db := range list {
		ids = append(ids, db.TeamID)
	}
	teams, err := h.views.TeamsByID(ctx, ids)
	names := make(map[string]string, len(teams))
	for id, t := range teams {
		names[id] = t.Name
	}
	return names, err
}

func (h DatabasesHandler) detail(ctx context.Context, a actor.Actor, environment, name string) (detailView, error) {
	db, err := h.databases.GetFor(ctx, a, environment, name)
	if err != nil {
		return detailView{}, err
	}
	views, err := h.databases.AttachmentsFor(ctx, a, environment, name)
	if err != nil {
		return detailView{}, err
	}
	names, err := h.teamNames(ctx, []entities.Database{db})
	if err != nil {
		return detailView{}, err
	}
	attachable, err := h.attachable(ctx, db, views)
	return detailView{
		Database: db, Team: names[db.TeamID], Address: h.address(), Attachments: attachmentsOf(views),
		Attachable: attachable, Blocker: blocker(views), CanEdit: h.canEdit(ctx, a, db),
	}, err
}

func (h DatabasesHandler) canEdit(ctx context.Context, a actor.Actor, db entities.Database) bool {
	return h.views.Can(ctx, a, authz.EditDatabase, authz.Resource{TeamID: db.TeamID, Environment: db.Environment})
}

func (h DatabasesHandler) address() string {
	e := h.databases.Endpoint()
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

func (h DatabasesHandler) attachable(ctx context.Context, db entities.Database, views []databases.AttachmentView) ([]string, error) {
	services, err := h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{db.TeamID}})
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	for _, slug := range slugsOf(views, activeOnly) {
		taken[slug] = true
	}
	free := []string{}
	for _, s := range services {
		if !taken[s.Slug] {
			free = append(free, s.Slug)
		}
	}
	return free, nil
}

func activeOnly(v databases.AttachmentView) bool {
	return v.DetachedAt == nil
}

func drainingOnly(v databases.AttachmentView) bool {
	return v.DetachedAt != nil && v.Draining
}

func slugsOf(views []databases.AttachmentView, keep func(databases.AttachmentView) bool) []string {
	slugs := []string{}
	for _, v := range views {
		if keep(v) {
			slugs = append(slugs, v.Service)
		}
	}
	return slugs
}

func blocker(views []databases.AttachmentView) string {
	parts := []string{}
	if active := slugsOf(views, activeOnly); len(active) > 0 {
		parts = append(parts, "Detach these first: "+strings.Join(active, ", ")+".")
	}
	if draining := slugsOf(views, drainingOnly); len(draining) > 0 {
		parts = append(parts, "Redeploy or remove: "+strings.Join(draining, ", ")+".")
	}
	return strings.Join(parts, " ")
}

func attachmentsOf(views []databases.AttachmentView) []Attachment {
	result := make([]Attachment, 0, len(views))
	for _, v := range views {
		result = append(result, Attachment{Service: v.Service, Since: since(v), State: stateOf(v), Detached: v.DetachedAt != nil})
	}
	return result
}

func since(v databases.AttachmentView) time.Time {
	if v.DetachedAt != nil {
		return *v.DetachedAt
	}
	return v.AttachedAt
}

func stateOf(v databases.AttachmentView) string {
	switch {
	case v.DetachedAt == nil:
		return "attached"
	case v.Draining:
		return "draining"
	}
	return "detached"
}

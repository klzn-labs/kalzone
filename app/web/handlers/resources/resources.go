package resources

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/app/web/handlers/databases"
	"github.com/klzn-labs/kalzone/app/web/handlers/queues"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const (
	queueKind    = "queue"
	databaseKind = "database"
)

type DatabaseSection interface {
	Rows(ctx context.Context, a actor.Actor, environment string) ([]databases.Row, error)
}

type QueueSection interface {
	Rows(ctx context.Context, environment string) ([]queues.Row, error)
}

type Section struct {
	Kind      string
	Title     string
	Noun      string
	NewHref   string
	Count     int
	Queues    []queues.Row
	Databases []databases.Row
}

type listView struct {
	Sections    []Section
	Environment string
	Kind        string
}

type ResourcesHandler struct {
	renderer  *web.Renderer
	queues    QueueSection
	databases DatabaseSection
}

func NewResourcesHandler(renderer *web.Renderer, queues QueueSection, databases DatabaseSection) ResourcesHandler {
	return ResourcesHandler{renderer: renderer, queues: queues, databases: databases}
}

func (h ResourcesHandler) List(c *gin.Context) {
	environment, kind := c.Query("environment"), c.Query("kind")
	sections, err := h.sections(c.Request.Context(), requestctx.ActorFrom(c), environment, kind)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "resources", web.Page{Title: "Resources", Nav: "resources", Data: listView{Sections: sections, Environment: environment, Kind: kind}})
}

func (h ResourcesHandler) sections(ctx context.Context, a actor.Actor, environment, kind string) ([]Section, error) {
	sections := []Section{}
	if kind == "" || kind == queueKind {
		section, err := h.queueSection(ctx, environment)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}
	if kind == "" || kind == databaseKind {
		section, err := h.databaseSection(ctx, a, environment)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}
	return sections, nil
}

func (h ResourcesHandler) databaseSection(ctx context.Context, a actor.Actor, environment string) (Section, error) {
	rows, err := h.databases.Rows(ctx, a, environment)
	if err != nil {
		return Section{}, err
	}
	return Section{Kind: databaseKind, Title: "Databases", Noun: "database", NewHref: "/resources/new/database", Count: len(rows), Databases: rows}, nil
}

func (h ResourcesHandler) queueSection(ctx context.Context, environment string) (Section, error) {
	rows, err := h.queues.Rows(ctx, environment)
	if err != nil {
		return Section{}, err
	}
	return Section{Kind: queueKind, Title: "Queues", Noun: "queue", NewHref: "/resources/new/queue", Count: len(rows), Queues: rows}, nil
}

func RegisterResourcesRoutes(routes httpserver.Web, h ResourcesHandler) {
	routes.GET("/resources", h.List)
	routes.GET("/resources/new", redirectTo("/resources/new/queue"))
	routes.GET("/queues", redirectTo("/resources?kind=queue"))
	routes.GET("/queues/*rest", redirectOldQueue)
}

func redirectOldQueue(c *gin.Context) {
	switch rest := c.Param("rest"); rest {
	case "", "/":
		redirectTo("/resources?kind=queue")(c)
	case "/new":
		redirectTo("/resources/new/queue")(c)
	default:
		redirectTo("/resources/queues" + rest)(c)
	}
}

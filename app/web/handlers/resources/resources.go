package resources

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	resources "github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/app/web/handlers/crons"
	"github.com/klzn-labs/kalzone/app/web/handlers/databases"
	"github.com/klzn-labs/kalzone/app/web/handlers/queues"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type DatabaseSection interface {
	Rows(ctx context.Context, a actor.Actor, environment string) ([]databases.Row, error)
}

type QueueSection interface {
	Rows(ctx context.Context, a actor.Actor, environment string) ([]queues.Row, error)
}

type CronSection interface {
	Rows(ctx context.Context, a actor.Actor, environment string) ([]crons.Row, error)
}

type Inventory interface {
	ListFor(ctx context.Context, a actor.Actor, kind resources.Kind, environment string) ([]resources.Resource, error)
}

type kindView struct {
	Section     Section
	Environment string
}

type ResourcesHandler struct {
	renderer  *web.Renderer
	queues    QueueSection
	databases DatabaseSection
	crons     CronSection
	inventory Inventory
}

func NewResourcesHandler(renderer *web.Renderer, queues QueueSection, databases DatabaseSection, crons CronSection, inventory Inventory) ResourcesHandler {
	return ResourcesHandler{renderer: renderer, queues: queues, databases: databases, crons: crons, inventory: inventory}
}

func (h ResourcesHandler) Catalog(c *gin.Context) {
	if info, ok := kinds[c.Query("kind")]; ok {
		redirectToKind(c, info)
		return
	}
	view, err := h.catalog(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "resources", web.Page{Title: "Resources", Nav: "resources", Data: view})
}

func (h ResourcesHandler) Counts(c *gin.Context) {
	view, err := h.catalog(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", nil)
		return
	}
	h.renderer.Fragment(c, http.StatusOK, "resources", "resource-counts", view)
}

func (h ResourcesHandler) kindPage(info kindInfo) gin.HandlerFunc {
	return func(c *gin.Context) {
		environment := c.Query("environment")
		section, err := h.section(c.Request.Context(), requestctx.ActorFrom(c), info, environment)
		if err != nil {
			web.ErrorPage(c, h.renderer, err)
			return
		}
		h.renderer.Render(c, http.StatusOK, "resource_kind", web.Page{Title: info.Title, Nav: info.Nav, Data: kindView{Section: section, Environment: environment}})
	}
}

func RegisterResourcesRoutes(routes httpserver.Web, h ResourcesHandler) {
	routes.GET("/resources", h.Catalog)
	routes.GET("/resources/counts", h.Counts)
	for _, info := range kinds {
		routes.GET(info.Href, h.kindPage(info))
	}
	routes.GET("/resources/new", redirectTo("/resources/new/queue"))
	routes.GET("/queues", redirectTo(kinds[queueKind].Href))
	routes.GET("/queues/*rest", redirectOldQueue)
}

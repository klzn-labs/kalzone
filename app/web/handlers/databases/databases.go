package databases

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Databases interface {
	Create(ctx context.Context, a actor.Actor, d databases.Draft) (entities.Database, error)
	Attach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error)
	Detach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error)
	Delete(ctx context.Context, a actor.Actor, environment, name string) error
	GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Database, error)
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Database, error)
	AttachmentsFor(ctx context.Context, a actor.Actor, environment, name string) ([]databases.AttachmentView, error)
	Endpoint() databases.Endpoint
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type DatabasesHandler struct {
	renderer  *web.Renderer
	views     web.Views
	databases Databases
	services  Services
	teams     Teams
}

func NewDatabasesHandler(renderer *web.Renderer, views web.Views, databases Databases, services Services, teams Teams) DatabasesHandler {
	return DatabasesHandler{renderer: renderer, views: views, databases: databases, services: services, teams: teams}
}

func (h DatabasesHandler) Show(c *gin.Context) {
	v, err := h.detail(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "database", web.Page{Title: v.Database.Name + " · " + v.Database.Environment, Nav: "resources", Data: v})
}

func (h DatabasesHandler) New(c *gin.Context) {
	teams, err := h.selectableTeams(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "database_new", web.Page{Title: "New database", Nav: "resources", Data: newView{Teams: teams, MaxConnections: 10}})
}

func (h DatabasesHandler) selectableTeams(ctx context.Context, a actor.Actor) ([]team.Team, error) {
	if a.Admin {
		return h.teams.List(ctx)
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	teams := make([]team.Team, 0, len(mine))
	for _, t := range mine {
		teams = append(teams, t.Team)
	}
	return teams, err
}

func RegisterDatabasesRoutes(routes httpserver.Web, h DatabasesHandler) {
	routes.GET("/resources/new/database", h.New)
	routes.POST("/resources/databases", h.Create)
	routes.GET("/resources/databases/:env/:name", h.Show)
	routes.POST("/resources/databases/:env/:name/attach", h.Attach)
	routes.POST("/resources/databases/:env/:name/detach/:service", h.Detach)
	routes.POST("/resources/databases/:env/:name/delete", h.Delete)
}

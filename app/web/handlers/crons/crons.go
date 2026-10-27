package crons

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	cronsUsecase "github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Crons interface {
	Create(ctx context.Context, a actor.Actor, d cronsUsecase.Draft) (entities.Cron, error)
	Update(ctx context.Context, a actor.Actor, environment, name string, change cronsUsecase.Change) (entities.Cron, error)
	SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Cron, error)
	Run(ctx context.Context, a actor.Actor, environment, name string) (entities.Command, error)
	Delete(ctx context.Context, a actor.Actor, environment, name string) error
	GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Cron, error)
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Cron, error)
	RunsFor(ctx context.Context, a actor.Actor, environment, name string) ([]entities.Run, error)
	LatestRun(ctx context.Context, c entities.Cron) (entities.Run, bool, error)
}

type ServiceNames interface {
	SlugOf(ctx context.Context, id string) (string, error)
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type CronsHandler struct {
	renderer *web.Renderer
	views    web.Views
	crons    Crons
	names    ServiceNames
	services Services
}

func NewCronsHandler(renderer *web.Renderer, views web.Views, crons Crons, names ServiceNames, services Services) CronsHandler {
	return CronsHandler{renderer: renderer, views: views, crons: crons, names: names, services: services}
}

func (h CronsHandler) Rows(ctx context.Context, a actor.Actor, environment string) ([]Row, error) {
	list, err := h.crons.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(list))
	for _, c := range list {
		rows = append(rows, h.row(ctx, c))
	}
	return rows, nil
}

func (h CronsHandler) Show(c *gin.Context) {
	v, err := h.detail(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	v.Edit = c.Query("edit") == "1"
	h.renderer.Render(c, http.StatusOK, "cron", web.Page{Title: v.Row.Cron.Name + " · " + v.Row.Cron.Environment, Nav: "crons", Data: v})
}

func (h CronsHandler) detail(ctx context.Context, a actor.Actor, environment, name string) (detailView, error) {
	c, err := h.crons.GetFor(ctx, a, environment, name)
	if err != nil {
		return detailView{}, err
	}
	runs, err := h.crons.RunsFor(ctx, a, environment, name)
	if err != nil {
		return detailView{}, err
	}
	canEdit := h.views.Can(ctx, a, authz.EditCron, authz.Resource{TeamID: c.TeamID, Environment: c.Environment})
	return detailView{Row: h.row(ctx, c), Runs: runRows(runs), NextRun: nextRun(c), Payload: prettyPayload(c.Body()), CanEdit: canEdit}, nil
}

func (h CronsHandler) New(c *gin.Context) {
	services, err := h.selectableServices(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "cron_new", web.Page{Title: "New cron", Nav: "crons", Data: newView{Services: services}})
}

func (h CronsHandler) selectableServices(ctx context.Context, a actor.Actor) ([]catalog.Service, error) {
	if a.Admin {
		return h.services.List(ctx, catalog.ServiceFilter{})
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	if err != nil || len(mine) == 0 {
		return nil, err
	}
	ids := make([]string, 0, len(mine))
	for _, t := range mine {
		ids = append(ids, t.Team.ID)
	}
	return h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: ids})
}

func RegisterCronsRoutes(routes httpserver.Web, h CronsHandler) {
	routes.GET("/resources/new/cron", h.New)
	routes.POST("/resources/crons", h.Create)
	routes.GET("/resources/crons/:env/:name", h.Show)
	routes.POST("/resources/crons/:env/:name/pause", h.Pause)
	routes.POST("/resources/crons/:env/:name/resume", h.Resume)
	routes.POST("/resources/crons/:env/:name/run", h.RunNow)
	routes.POST("/resources/crons/:env/:name/settings", h.Edit)
	routes.POST("/resources/crons/:env/:name/delete", h.Delete)
}

package queues

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Queues interface {
	Create(ctx context.Context, a actor.Actor, d queues.Draft) (entities.Queue, error)
	Update(ctx context.Context, a actor.Actor, environment, name string, c queues.Change) (entities.Queue, error)
	SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error)
	Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error
	GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error)
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Queue, error)
	Versions(ctx context.Context, environment, name string) ([]entities.Version, error)
	Stats(ctx context.Context, environment, name string) (entities.Stats, bool, error)
	Replay(ctx context.Context, a actor.Actor, environment, name, messageID string) error
	ReplayAll(ctx context.Context, a actor.Actor, environment, name string) error
	PurgeDead(ctx context.Context, a actor.Actor, environment, name string) error
}

type ServiceNames interface {
	SlugOf(ctx context.Context, id string) (string, error)
}

type QueuesHandler struct {
	renderer *web.Renderer
	queues   Queues
	names    ServiceNames
	views    web.Views
	grafana  web.GrafanaOrgs
}

func NewQueuesHandler(renderer *web.Renderer, queues Queues, names ServiceNames, views web.Views, grafana web.GrafanaOrgs) QueuesHandler {
	return QueuesHandler{renderer: renderer, queues: queues, names: names, views: views, grafana: grafana}
}

func (h QueuesHandler) Rows(ctx context.Context, a actor.Actor, environment string) ([]Row, error) {
	list, err := h.queues.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(list))
	for _, q := range list {
		rows = append(rows, h.row(ctx, q))
	}
	return rows, nil
}

func (h QueuesHandler) Show(c *gin.Context) {
	q, err := h.queues.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	versions, err := h.queues.Versions(c.Request.Context(), q.Environment, q.Name)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	v := detailView{Row: h.row(c.Request.Context(), q), Tab: tab(c.Query("tab")), Edit: c.Query("edit") == "1", Versions: versions, Charts: h.charts(c.Request.Context(), q)}
	h.renderer.Render(c, http.StatusOK, "queue", web.Page{Title: q.Name + " · " + q.Environment, Nav: "resources", Data: v})
}

func (h QueuesHandler) charts(ctx context.Context, q entities.Queue) web.Charts {
	teams, err := h.views.TeamsByID(ctx, []string{q.TeamID})
	if err != nil {
		return web.Charts{}
	}
	return web.QueueCharts(h.grafana, teams[q.TeamID].Slug, q.ID)
}

func (h QueuesHandler) New(c *gin.Context) {
	h.renderer.Render(c, http.StatusOK, "queue_new", web.Page{Title: "New queue", Nav: "resources", Data: entities.DefaultSettings("/hooks/")})
}

func RegisterQueuesRoutes(routes httpserver.Web, h QueuesHandler) {
	routes.GET("/resources/new/queue", h.New)
	routes.POST("/resources/queues", h.Create)
	routes.GET("/resources/queues/:env/:name", h.Show)
	routes.POST("/resources/queues/:env/:name/pause", h.Pause)
	routes.POST("/resources/queues/:env/:name/resume", h.Resume)
	routes.POST("/resources/queues/:env/:name/settings", h.Edit)
	routes.POST("/resources/queues/:env/:name/delete", h.Delete)
	routes.POST("/resources/queues/:env/:name/dead/replay-all", h.ReplayAll)
	routes.POST("/resources/queues/:env/:name/dead/purge", h.Purge)
	routes.POST("/resources/queues/:env/:name/dead/:id/replay", h.Replay)
}

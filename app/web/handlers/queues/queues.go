package queues

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Queues interface {
	Create(ctx context.Context, a actor.Actor, d queues.Draft) (entities.Queue, error)
	Update(ctx context.Context, a actor.Actor, environment, name string, c queues.Change) (entities.Queue, error)
	SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error)
	Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error
	Get(ctx context.Context, environment, name string) (entities.Queue, error)
	List(ctx context.Context, environment, teamSlug string) ([]entities.Queue, error)
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
}

func NewQueuesHandler(renderer *web.Renderer, queues Queues, names ServiceNames) QueuesHandler {
	return QueuesHandler{renderer: renderer, queues: queues, names: names}
}

func (h QueuesHandler) List(c *gin.Context) {
	list, err := h.queues.List(c.Request.Context(), c.Query("environment"), "")
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	rows := make([]Row, 0, len(list))
	for _, q := range list {
		rows = append(rows, h.row(c.Request.Context(), q))
	}
	h.renderer.Render(c, http.StatusOK, "queues", web.Page{Title: "Queues", Nav: "queues", Data: listView{Rows: rows, Environment: c.Query("environment")}})
}

func (h QueuesHandler) Show(c *gin.Context) {
	q, err := h.queues.Get(c.Request.Context(), c.Param("env"), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	versions, err := h.queues.Versions(c.Request.Context(), q.Environment, q.Name)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	v := detailView{Row: h.row(c.Request.Context(), q), Tab: tab(c.Query("tab")), Edit: c.Query("edit") == "1", Versions: versions}
	h.renderer.Render(c, http.StatusOK, "queue", web.Page{Title: q.Name + " · " + q.Environment, Nav: "queues", Data: v})
}

func (h QueuesHandler) New(c *gin.Context) {
	h.renderer.Render(c, http.StatusOK, "queue_new", web.Page{Title: "Create queue", Nav: "queues", Data: entities.DefaultSettings("/hooks/")})
}

func RegisterQueuesRoutes(routes httpserver.Web, h QueuesHandler) {
	routes.GET("/queues", h.List)
	routes.GET("/queues/new", h.New)
	routes.POST("/queues", h.Create)
	routes.GET("/queues/:env/:name", h.Show)
	routes.POST("/queues/:env/:name/pause", h.Pause)
	routes.POST("/queues/:env/:name/resume", h.Resume)
	routes.POST("/queues/:env/:name/settings", h.Edit)
	routes.POST("/queues/:env/:name/delete", h.Delete)
	routes.POST("/queues/:env/:name/dead/replay-all", h.ReplayAll)
	routes.POST("/queues/:env/:name/dead/purge", h.Purge)
	routes.POST("/queues/:env/:name/dead/:id/replay", h.Replay)
}

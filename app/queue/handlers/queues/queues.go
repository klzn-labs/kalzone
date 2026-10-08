package queues

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
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
	queues Queues
	names  ServiceNames
}

func NewQueuesHandler(queues Queues, names ServiceNames) QueuesHandler {
	return QueuesHandler{queues: queues, names: names}
}

func (h QueuesHandler) List(c *gin.Context) {
	list, err := h.queues.List(c.Request.Context(), c.Query("environment"), c.Query("team"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]QueueDTO, 0, len(list))
	for _, q := range list {
		result = append(result, h.toDTO(c.Request.Context(), q))
	}
	c.JSON(http.StatusOK, gin.H{"queues": result})
}

func (h QueuesHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	settings, err := body.settings()
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	q, err := h.queues.Create(c.Request.Context(), requestctx.ActorFrom(c), queues.Draft{
		Name: body.Name, Environment: body.Environment, Consumer: body.Consumer,
		Settings: settings, Publishers: body.Publishers, Message: body.Message,
	})
	h.respond(c, http.StatusCreated, q, err)
}

func (h QueuesHandler) Get(c *gin.Context) {
	q, err := h.queues.Get(c.Request.Context(), c.Param("env"), c.Param("name"))
	h.respond(c, http.StatusOK, q, err)
}

func (h QueuesHandler) Update(c *gin.Context) {
	var body changeDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	settings, err := body.settings()
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	change := queues.Change{Settings: settings, Publishers: body.Publishers, Message: body.Message, BaseVersion: body.Version}
	q, err := h.queues.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), change)
	h.respond(c, http.StatusOK, q, err)
}

func (h QueuesHandler) Delete(c *gin.Context) {
	err := h.queues.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.Query("force") == "true")
	noContent(c, http.StatusNoContent, err)
}

func (h QueuesHandler) Pause(c *gin.Context) {
	q, err := h.queues.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), true)
	h.respond(c, http.StatusOK, q, err)
}

func (h QueuesHandler) Resume(c *gin.Context) {
	q, err := h.queues.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), false)
	h.respond(c, http.StatusOK, q, err)
}

func (h QueuesHandler) respond(c *gin.Context, status int, q entities.Queue, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(status, h.toDTO(c.Request.Context(), q))
}

func (h QueuesHandler) toDTO(ctx context.Context, q entities.Queue) QueueDTO {
	consumer, _ := h.names.SlugOf(ctx, q.ConsumerID)
	return QueueDTO{
		ID: q.ID, Name: q.Name, Environment: q.Environment, TeamID: q.TeamID, Consumer: consumer,
		Publishers: h.slugs(ctx, q.Publishers), Version: q.Version, Settings: fromSettings(q.Settings),
		Paused: q.Paused, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
}

func (h QueuesHandler) slugs(ctx context.Context, ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if slug, err := h.names.SlugOf(ctx, id); err == nil {
			result = append(result, slug)
		}
	}
	return result
}

func noContent(c *gin.Context, status int, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(status)
}

func RegisterQueuesRoutes(api httpserver.API, h QueuesHandler) {
	api.GET("/queues", h.List)
	api.POST("/queues", h.Create)
	queue := api.Group("/queues/:env/:name")
	queue.GET("", h.Get)
	queue.PATCH("", h.Update)
	queue.DELETE("", h.Delete)
	queue.POST("/pause", h.Pause)
	queue.POST("/resume", h.Resume)
	queue.GET("/stats", h.Stats)
	queue.GET("/versions", h.Versions)
	queue.GET("/dead", h.Dead)
	queue.DELETE("/dead", h.PurgeDead)
	queue.POST("/dead/replay-all", h.ReplayAll)
	queue.POST("/dead/:id/replay", h.Replay)
}

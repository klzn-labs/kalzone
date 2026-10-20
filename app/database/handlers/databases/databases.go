package databases

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Databases interface {
	Create(ctx context.Context, a actor.Actor, d databases.Draft) (entities.Database, error)
	Adopt(ctx context.Context, a actor.Actor, d databases.AdoptDraft) (entities.Database, error)
	Attach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error)
	Detach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error)
	Delete(ctx context.Context, a actor.Actor, environment, name string) error
	GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Database, error)
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Database, error)
	AttachmentsFor(ctx context.Context, a actor.Actor, environment, name string) ([]databases.AttachmentView, error)
	Endpoint() databases.Endpoint
}

type DatabasesHandler struct {
	databases Databases
}

func NewDatabasesHandler(databases Databases) DatabasesHandler {
	return DatabasesHandler{databases: databases}
}

func (h DatabasesHandler) List(c *gin.Context) {
	load := func() ([]described, error) {
		return h.list(c.Request.Context(), requestctx.ActorFrom(c), c.Query("environment"), c.Query("team"))
	}
	endpoint := h.databases.Endpoint()
	apispec.RespondWindow(c, load, describedKey, func(d described) DatabaseDTO { return toDTO(d, endpoint) })
}

func (h DatabasesHandler) list(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]described, error) {
	all, err := h.databases.ListFor(ctx, a, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	result := make([]described, 0, len(all))
	for _, db := range all {
		d, err := h.describe(ctx, a, db)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

func (h DatabasesHandler) describe(ctx context.Context, a actor.Actor, db entities.Database) (described, error) {
	attachments, err := h.databases.AttachmentsFor(ctx, a, db.Environment, db.Name)
	return described{database: db, attachments: attachments}, err
}

func (h DatabasesHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	draft := databases.Draft{Name: body.Name, Environment: body.Environment, Team: body.Team, MaxConnections: body.MaxConnections}
	db, err := h.databases.Create(c.Request.Context(), requestctx.ActorFrom(c), draft)
	h.respond(c, http.StatusCreated, db, err, "")
}

func (h DatabasesHandler) Adopt(c *gin.Context) {
	var body adoptDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	draft := databases.AdoptDraft{
		Schema: body.Schema, Name: body.Name, Environment: body.Environment, Team: body.Team,
		MaxConnections: body.MaxConnections, Services: body.Services,
	}
	db, err := h.databases.Adopt(c.Request.Context(), requestctx.ActorFrom(c), draft)
	h.respond(c, http.StatusCreated, db, err, adoptedNote)
}

func (h DatabasesHandler) Get(c *gin.Context) {
	db, err := h.databases.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.respond(c, http.StatusOK, db, err, "")
}

func (h DatabasesHandler) Attach(c *gin.Context) {
	var body attachDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	db, err := h.databases.Attach(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), body.Service)
	h.respond(c, http.StatusOK, db, err, nextDeployNote)
}

func (h DatabasesHandler) Detach(c *gin.Context) {
	db, err := h.databases.Detach(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.Param("service"))
	h.respond(c, http.StatusOK, db, err, nextDeployNote)
}

func (h DatabasesHandler) Delete(c *gin.Context) {
	ctx, a := c.Request.Context(), requestctx.ActorFrom(c)
	db, err := h.databases.GetFor(ctx, a, c.Param("env"), c.Param("name"))
	if err == nil {
		err = h.databases.Delete(ctx, a, db.Environment, db.Name)
	}
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	if db.Adopted {
		c.JSON(http.StatusOK, releasedDTO{Released: true, Message: releasedMessage})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h DatabasesHandler) respond(c *gin.Context, status int, db entities.Database, err error, note string) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	d, err := h.describe(c.Request.Context(), requestctx.ActorFrom(c), db)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	dto := toDTO(d, h.databases.Endpoint())
	dto.Note = note
	c.JSON(status, dto)
}

func RegisterDatabasesRoutes(api httpserver.API, h DatabasesHandler) {
	api.Handle(databaseOperations(h)...)
}

package crons

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

type Crons interface {
	Create(ctx context.Context, a actor.Actor, d crons.Draft) (entities.Cron, error)
	Update(ctx context.Context, a actor.Actor, environment, name string, c crons.Change) (entities.Cron, error)
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

type CronsHandler struct {
	crons Crons
	names ServiceNames
}

func NewCronsHandler(crons Crons, names ServiceNames) CronsHandler {
	return CronsHandler{crons: crons, names: names}
}

func (h CronsHandler) List(c *gin.Context) {
	load := func() ([]described, error) {
		return h.list(c.Request.Context(), requestctx.ActorFrom(c), c.Query("environment"), c.Query("team"))
	}
	now := clock.Now().UTC()
	apispec.RespondWindow(c, load, describedKey, func(d described) CronDTO { return toDTO(d, now) })
}

func (h CronsHandler) list(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]described, error) {
	all, err := h.crons.ListFor(ctx, a, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	result := make([]described, 0, len(all))
	for _, cron := range all {
		d, err := h.describe(ctx, cron)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

func (h CronsHandler) describe(ctx context.Context, cron entities.Cron) (described, error) {
	service, _ := h.names.SlugOf(ctx, cron.ServiceID)
	run, found, err := h.crons.LatestRun(ctx, cron)
	if err != nil || !found {
		return described{cron: cron, service: service}, err
	}
	return described{cron: cron, service: service, lastRun: &run}, nil
}

func (h CronsHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	draft, err := body.draft()
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	cron, err := h.crons.Create(c.Request.Context(), requestctx.ActorFrom(c), draft)
	h.respond(c, http.StatusCreated, cron, err)
}

func (h CronsHandler) Get(c *gin.Context) {
	cron, err := h.crons.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.respond(c, http.StatusOK, cron, err)
}

func (h CronsHandler) Update(c *gin.Context) {
	var body changeDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	change, err := body.change()
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	cron, err := h.crons.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), change)
	h.respond(c, http.StatusOK, cron, err)
}

func (h CronsHandler) Delete(c *gin.Context) {
	err := h.crons.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h CronsHandler) respond(c *gin.Context, status int, cron entities.Cron, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	d, err := h.describe(c.Request.Context(), cron)
	apispec.Respond(c, status, toDTO(d, clock.Now().UTC()), err)
}

func RegisterCronsRoutes(api httpserver.API, h CronsHandler) {
	api.Handle(cronOperations(h)...)
	api.Handle(controlOperations(h)...)
}

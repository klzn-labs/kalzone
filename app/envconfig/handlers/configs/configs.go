package configs

import (
	"context"
	"net/http"
	"strconv"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Configs interface {
	CurrentFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Revision, error)
	HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Revision, error)
	Revise(ctx context.Context, a actor.Actor, serviceSlug, environment string, cs entities.ChangeSet) (entities.Revision, error)
	SetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name, value string) (entities.Revision, error)
	UnsetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name string) (entities.Revision, error)
	SecretsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) ([]entities.SecretInfo, error)
}

type ConfigsHandler struct {
	configs Configs
}

func NewConfigsHandler(configs Configs) ConfigsHandler {
	return ConfigsHandler{configs: configs}
}

func (h ConfigsHandler) Current(c *gin.Context) {
	rev, err := h.configs.CurrentFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	respond(c, http.StatusOK, rev, err)
}

func (h ConfigsHandler) History(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	revisions, err := h.configs.HistoryFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), limit)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]revisionDTO, 0, len(revisions))
	for _, r := range revisions {
		result = append(result, toDTO(r))
	}
	c.JSON(http.StatusOK, gin.H{"revisions": result})
}

func (h ConfigsHandler) Revise(c *gin.Context) {
	var body changeDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	rev, err := h.configs.Revise(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), body.toEntity())
	respond(c, http.StatusCreated, rev, err)
}

func (h ConfigsHandler) SetSecret(c *gin.Context) {
	var body secretDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	rev, err := h.configs.SetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), c.Param("name"), body.Value)
	respond(c, http.StatusOK, rev, err)
}

func (h ConfigsHandler) UnsetSecret(c *gin.Context) {
	rev, err := h.configs.UnsetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), c.Param("name"))
	respond(c, http.StatusOK, rev, err)
}

func (h ConfigsHandler) Secrets(c *gin.Context) {
	infos, err := h.configs.SecretsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]secretInfoDTO, 0, len(infos))
	for _, i := range infos {
		result = append(result, secretInfoDTO{Name: i.Name, SetBy: i.SetBy, SetAt: i.SetAt})
	}
	c.JSON(http.StatusOK, gin.H{"secrets": result})
}

func respond(c *gin.Context, status int, rev entities.Revision, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(status, toDTO(rev))
}

func RegisterConfigsRoutes(api httpserver.API, h ConfigsHandler) {
	api.GET("/config/:service/:env", h.Current)
	api.GET("/config/:service/:env/revisions", h.History)
	api.POST("/config/:service/:env/revisions", h.Revise)
	api.GET("/config/:service/:env/secrets", h.Secrets)
	api.PUT("/config/:service/:env/secrets/:name", h.SetSecret)
	api.DELETE("/config/:service/:env/secrets/:name", h.UnsetSecret)
}

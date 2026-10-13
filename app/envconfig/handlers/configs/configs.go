package configs

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Configs interface {
	CurrentFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Revision, error)
	RevisionsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, p page.Request) (page.Page[entities.Revision], error)
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
	apispec.Respond(c, http.StatusOK, toDTO(rev), err)
}

func (h ConfigsHandler) History(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Revision], error) {
		return h.configs.RevisionsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), p)
	}
	apispec.RespondPage(c, load, toDTO)
}

func (h ConfigsHandler) Revise(c *gin.Context) {
	var body changeDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	rev, err := h.configs.Revise(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), body.toEntity())
	apispec.Respond(c, http.StatusCreated, toDTO(rev), err)
}

func (h ConfigsHandler) SetSecret(c *gin.Context) {
	var body secretDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	rev, err := h.configs.SetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), c.Param("name"), body.Value)
	apispec.Respond(c, http.StatusOK, toDTO(rev), err)
}

func (h ConfigsHandler) UnsetSecret(c *gin.Context) {
	rev, err := h.configs.UnsetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), c.Param("name"))
	apispec.Respond(c, http.StatusOK, toDTO(rev), err)
}

func (h ConfigsHandler) Secrets(c *gin.Context) {
	load := func() ([]entities.SecretInfo, error) {
		return h.configs.SecretsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	}
	apispec.RespondWindow(c, load, secretKey, toSecretDTO)
}

func RegisterConfigsRoutes(api httpserver.API, h ConfigsHandler) {
	api.Handle(
		apispec.Operation{ID: "getConfig", Method: http.MethodGet, Path: "/config/:service/:env", Summary: "Show the current config revision; secrets by name only",
			Scope: scope.Read, Params: configParams{}, Status: http.StatusOK, Response: revisionDTO{}, Handler: h.Current},
		apispec.Operation{ID: "listRevisions", Method: http.MethodGet, Path: "/config/:service/:env/revisions", Summary: "List config revisions, newest first",
			Scope: scope.Read, Params: pagedParams{}, Status: http.StatusOK, Response: apispec.Page[revisionDTO]{}, Handler: h.History},
		apispec.Operation{ID: "reviseConfig", Method: http.MethodPost, Path: "/config/:service/:env/revisions", Summary: "Write a new revision; a spec replaces the whole spec",
			Scope: scope.Config, Gated: true, Params: configParams{}, Body: changeDTO{}, Status: http.StatusCreated, Response: revisionDTO{}, Handler: h.Revise},
		apispec.Operation{ID: "listSecrets", Method: http.MethodGet, Path: "/config/:service/:env/secrets", Summary: "List secret names and who set them",
			Scope: scope.Read, Params: pagedParams{}, Status: http.StatusOK, Response: apispec.Page[secretInfoDTO]{}, Handler: h.Secrets},
		apispec.Operation{ID: "setSecret", Method: http.MethodPut, Path: "/config/:service/:env/secrets/:name", Summary: "Set a secret; its value never comes back",
			Scope: scope.Secrets, Gated: true, Params: secretParams{}, Body: secretDTO{}, Status: http.StatusOK, Response: revisionDTO{}, Handler: h.SetSecret},
		apispec.Operation{ID: "unsetSecret", Method: http.MethodDelete, Path: "/config/:service/:env/secrets/:name", Summary: "Remove a secret",
			Scope: scope.Secrets, Gated: true, Params: secretParams{}, Status: http.StatusOK, Response: revisionDTO{}, Handler: h.UnsetSecret},
	)
}

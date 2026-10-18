package environments

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Environments interface {
	List(ctx context.Context) ([]entities.Environment, error)
}

type EnvironmentsHandler struct {
	environments Environments
}

func NewEnvironmentsHandler(environments Environments) EnvironmentsHandler {
	return EnvironmentsHandler{environments: environments}
}

type environmentDTO struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

func toDTO(e entities.Environment) environmentDTO {
	return environmentDTO{Slug: e.Slug, Name: e.Name, Protected: e.Protected}
}

func environmentKey(e entities.Environment) string {
	return e.Slug
}

func (h EnvironmentsHandler) List(c *gin.Context) {
	apispec.RespondWindow(c, func() ([]entities.Environment, error) { return h.environments.List(c.Request.Context()) }, environmentKey, toDTO)
}

func RegisterEnvironmentsRoutes(api httpserver.API, h EnvironmentsHandler) {
	api.Handle(apispec.Operation{
		ID: "listEnvironments", Method: http.MethodGet, Path: "/environments", Summary: "List environments in promotion order; protected ones need approval for agents",
		Scope: scope.Read, Params: apispec.ListParams{}, Status: http.StatusOK, Response: apispec.Page[environmentDTO]{}, Handler: h.List,
	})
}

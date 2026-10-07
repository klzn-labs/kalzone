package environments

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/gin-gonic/gin"
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

func (h EnvironmentsHandler) List(c *gin.Context) {
	list, err := h.environments.List(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]environmentDTO, 0, len(list))
	for _, e := range list {
		result = append(result, environmentDTO{Slug: e.Slug, Name: e.Name, Protected: e.Protected})
	}
	c.JSON(http.StatusOK, gin.H{"environments": result})
}

func RegisterEnvironmentsRoutes(api httpserver.API, h EnvironmentsHandler) {
	api.GET("/environments", h.List)
}

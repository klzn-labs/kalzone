package resources

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Resources interface {
	ListFor(ctx context.Context, a actor.Actor, kind entities.Kind, environment string) ([]entities.Resource, error)
}

type ResourcesHandler struct {
	resources Resources
}

func NewResourcesHandler(resources Resources) ResourcesHandler {
	return ResourcesHandler{resources: resources}
}

func (h ResourcesHandler) List(c *gin.Context) {
	load := func() ([]entities.Resource, error) {
		return h.resources.ListFor(c.Request.Context(), requestctx.ActorFrom(c), entities.Kind(c.Query("kind")), c.Query("environment"))
	}
	apispec.RespondWindow(c, load, resourceKey, toDTO)
}

func RegisterResourcesRoutes(api httpserver.API, h ResourcesHandler) {
	api.Handle(resourceOperations(h)...)
}

package environment

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/internal/requestctx"
)

func (h EnvironmentHandler) Panel(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	if err != nil {
		c.Status(http.StatusNoContent)
		return
	}
	h.renderer.Fragment(c, http.StatusOK, "environment", "deploy-panel", map[string]any{"Data": d, "CSRF": requestctx.ViewerFrom(c).CSRFToken})
}

func (h EnvironmentHandler) Deploy(c *gin.Context) {
	request := deployments.Request{
		Service: c.Param("service"), Environment: c.Param("env"), ReleaseID: c.PostForm("release_id"), Strategy: c.PostForm("strategy"),
	}
	d, err := h.deployments.Deploy(c.Request.Context(), requestctx.ActorFrom(c), request)
	h.finish(c, err, fmt.Sprintf("Deployment #%d queued", d.Number))
}

func (h EnvironmentHandler) ApplyConfig(c *gin.Context) {
	d, err := h.deployments.ApplyConfig(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	h.finish(c, err, fmt.Sprintf("Deployment #%d queued with config c%d", d.Number, d.ConfigRevision))
}

func (h EnvironmentHandler) Promote(c *gin.Context) {
	d, err := h.deployments.Promote(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.PostForm("from"), c.Param("env"))
	h.finish(c, err, fmt.Sprintf("Promoting r%d, deployment #%d queued", d.ReleaseNumber, d.Number))
}

func (h EnvironmentHandler) Rollback(c *gin.Context) {
	d, err := h.deployments.Rollback(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.finish(c, err, fmt.Sprintf("Rolling back to r%d with config c%d", d.ReleaseNumber, d.ConfigRevision))
}

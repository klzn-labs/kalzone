package agentapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

func (h AgentAPIHandler) Platform(c *gin.Context) {
	var body agentapi.PlatformReport
	if !httperror.BindJSON(c, &body) {
		return
	}
	if err := h.agents.ReportPlatform(c.Request.Context(), agentName(c), clock.Now().UTC(), componentOf(c.Param("name"), body)); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func componentOf(name string, report agentapi.PlatformReport) entities.PlatformComponent {
	history := make([]entities.PlatformDeploy, 0, len(report.History))
	for _, d := range report.History {
		history = append(history, entities.PlatformDeploy(d))
	}
	return entities.PlatformComponent{
		Name: name, Live: entities.PlatformRelease(report.Live), Container: platformContainerOf(report.Container), History: history,
	}
}

func platformContainerOf(c agentapi.Container) entities.PlatformContainer {
	return entities.PlatformContainer{
		Found: c.Found, Name: c.Name, State: c.State, Health: c.Health, Restarts: c.Restarts,
		StartedAt: c.StartedAt.UTC(), MemoryLimitMB: c.MemoryLimitMB,
	}
}

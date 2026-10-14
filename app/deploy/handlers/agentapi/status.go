package agentapi

import (
	"net/http"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/gin-gonic/gin"
)

func (h AgentAPIHandler) Status(c *gin.Context) {
	var body agentapi.StatusReport
	if !httperror.BindJSON(c, &body) {
		return
	}
	if err := h.agents.ReportStatus(c.Request.Context(), agentName(c), clock.Now().UTC(), snapshotsOf(body)); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func snapshotsOf(report agentapi.StatusReport) []entities.Snapshot {
	snapshots := make([]entities.Snapshot, 0, len(report.Workloads))
	for _, w := range report.Workloads {
		snapshots = append(snapshots, entities.Snapshot{Service: w.Service, Environment: w.Environment, Status: statusOf(w.Container)})
	}
	return snapshots
}

func statusOf(c agentapi.Container) entities.RuntimeStatus {
	status := entities.RuntimeStatus{
		Container: c.Name, Found: c.Found, State: c.State, Health: c.Health, HealthLog: c.HealthLog, ExitCode: c.ExitCode,
		OOMKilled: c.OOMKilled, Restarts: c.Restarts, Image: c.Image, MemoryLimitMB: c.MemoryLimitMB, CPULimitMillis: c.CPULimitMillis,
	}
	if !c.StartedAt.IsZero() {
		started := c.StartedAt.UTC()
		status.StartedAt = &started
	}
	return status
}

func failureOf(f *agentapi.Failure) *entities.Failure {
	if f == nil {
		return nil
	}
	return &entities.Failure{Class: f.Class, Container: f.Container, ExitCode: f.ExitCode, OOMKilled: f.OOMKilled, HealthLog: f.HealthLog, Logs: f.Logs}
}

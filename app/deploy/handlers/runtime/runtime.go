package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Runtime interface {
	StatusFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.RuntimeView, error)
}

type runtimeParams struct {
	Service string `path:"service" description:"service slug"`
	Env     string `path:"env" description:"environment slug"`
}

type usageDTO struct {
	CPUMillis      int `json:"cpu_millis"`
	CPULimitMillis int `json:"cpu_limit_millis"`
	MemoryMB       int `json:"memory_mb"`
	MemoryLimitMB  int `json:"memory_limit_mb"`
}

type RuntimeDTO struct {
	Service     string     `json:"service"`
	Environment string     `json:"environment"`
	Container   string     `json:"container"`
	Found       bool       `json:"found"`
	State       string     `json:"state" description:"running, exited, restarting, created, paused or dead; empty when not found"`
	Health      string     `json:"health" description:"healthy, unhealthy, starting, or empty without a health check"`
	HealthLog   []string   `json:"health_log"`
	Restarts    int        `json:"restarts"`
	ExitCode    int        `json:"exit_code"`
	OOMKilled   bool       `json:"oom_killed"`
	Image       string     `json:"image"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	ObservedAt  time.Time  `json:"observed_at"`
	Stale       bool       `json:"stale" description:"the agent has not reported for over 60s"`
	Usage       *usageDTO  `json:"usage,omitempty"`
}

type RuntimeHandler struct {
	runtime Runtime
}

func NewRuntimeHandler(runtime Runtime) RuntimeHandler {
	return RuntimeHandler{runtime: runtime}
}

func RegisterRuntimeRoutes(api httpserver.API, h RuntimeHandler) {
	api.Handle(apispec.Operation{
		ID: "getRuntime", Method: http.MethodGet, Path: "/runtime/:service/:env",
		Summary: "Show the live container of a service in one environment: state, health, restarts, CPU and memory",
		Scope:   scope.Read, Params: runtimeParams{}, Status: http.StatusOK, Response: RuntimeDTO{}, Handler: h.Get,
	})
}

func (h RuntimeHandler) Get(c *gin.Context) {
	view, err := h.runtime.StatusFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	apispec.Respond(c, http.StatusOK, toDTO(view), err)
}

func toDTO(v entities.RuntimeView) RuntimeDTO {
	s := v.Status
	dto := RuntimeDTO{
		Service: v.Service, Environment: v.Environment, Container: s.Container, Found: s.Found, State: s.State, Health: s.Health,
		HealthLog: nonNil(s.HealthLog), Restarts: s.Restarts, ExitCode: s.ExitCode, OOMKilled: s.OOMKilled, Image: s.Image,
		StartedAt: s.StartedAt, ObservedAt: s.ObservedAt, Stale: v.Stale,
	}
	if v.Usage != nil {
		dto.Usage = &usageDTO{CPUMillis: v.Usage.CPUMillis, CPULimitMillis: v.Usage.CPULimitMillis, MemoryMB: v.Usage.MemoryMB, MemoryLimitMB: v.Usage.MemoryLimitMB}
	}
	return dto
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}

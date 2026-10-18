package onboarding

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/onboarding/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Onboarding interface {
	Check(ctx context.Context, a actor.Actor, slug string) (entities.Report, error)
	OpenPullRequest(ctx context.Context, a actor.Actor, slug string) (entities.OpenedPullRequest, error)
}

type serviceParams struct {
	Service string `path:"service" description:"service slug"`
}

type checkDTO struct {
	Name   string `json:"name" enum:"github_app,workflow,dockerfile,registry,config"`
	Status string `json:"status" enum:"ok,failed,skipped"`
	Detail string `json:"detail"`
	Hint   string `json:"hint"`
}

type ReportDTO struct {
	Service string     `json:"service"`
	Ready   bool       `json:"ready" description:"every check is ok"`
	Checks  []checkDTO `json:"checks"`
}

type pullRequestDTO struct {
	URL    string `json:"url"`
	Number int    `json:"number"`
	Branch string `json:"branch"`
}

type OnboardingHandler struct {
	onboarding Onboarding
}

func NewOnboardingHandler(onboarding Onboarding) OnboardingHandler {
	return OnboardingHandler{onboarding: onboarding}
}

func RegisterOnboardingRoutes(api httpserver.API, h OnboardingHandler) {
	api.Handle(
		apispec.Operation{ID: "getOnboarding", Method: http.MethodGet, Path: "/services/:service/onboarding",
			Summary: "Check what a service still needs before it can build and deploy: GitHub App, build workflow, Dockerfile, registry and dev config",
			Scope:   scope.Read, Params: serviceParams{}, Status: http.StatusOK, Response: ReportDTO{}, Handler: h.Check},
		apispec.Operation{ID: "createOnboardingPullRequest", Method: http.MethodPost, Path: "/services/:service/onboarding/pull-request",
			Summary: "Open a pull request that adds the platform build workflow to the service's repository",
			Scope:   scope.Services, Params: serviceParams{}, Status: http.StatusCreated, Response: pullRequestDTO{}, Handler: h.OpenPullRequest},
	)
}

func (h OnboardingHandler) Check(c *gin.Context) {
	report, err := h.onboarding.Check(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	apispec.Respond(c, http.StatusOK, toReportDTO(report), err)
}

func (h OnboardingHandler) OpenPullRequest(c *gin.Context) {
	opened, err := h.onboarding.OpenPullRequest(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	apispec.Respond(c, http.StatusCreated, pullRequestDTO(opened), err)
}

func toReportDTO(r entities.Report) ReportDTO {
	checks := make([]checkDTO, 0, len(r.Checks))
	for _, c := range r.Checks {
		checks = append(checks, checkDTO(c))
	}
	return ReportDTO{Service: r.Service, Ready: r.Ready, Checks: checks}
}

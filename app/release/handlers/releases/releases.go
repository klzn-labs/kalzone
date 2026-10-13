package releases

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Releases interface {
	PageFor(ctx context.Context, a actor.Actor, serviceSlug string, p page.Request) (page.Page[entities.Release], error)
	GetFor(ctx context.Context, a actor.Actor, id string) (entities.Release, error)
	Request(ctx context.Context, a actor.Actor, serviceSlug, ref string) (entities.Release, error)
	Register(ctx context.Context, a actor.Actor, serviceSlug, tag string) (entities.Release, error)
}

type ReleasesHandler struct {
	releases Releases
}

func NewReleasesHandler(releases Releases) ReleasesHandler {
	return ReleasesHandler{releases: releases}
}

type ReleaseDTO struct {
	ID         string     `json:"id"`
	Number     int        `json:"number"`
	Source     string     `json:"source" enum:"build,registered,shared"`
	Status     string     `json:"status" enum:"requested,building,built,failed"`
	Trigger    string     `json:"trigger" enum:"manual,push,register"`
	GitRef     string     `json:"git_ref,omitempty"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	Image      string     `json:"image"`
	Tag        string     `json:"tag,omitempty"`
	Digest     string     `json:"digest,omitempty"`
	Reference  string     `json:"reference,omitempty"`
	RunURL     string     `json:"run_url,omitempty"`
	Failure    string     `json:"failure,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func ToDTO(r entities.Release) ReleaseDTO {
	return ReleaseDTO{
		ID: r.ID, Number: r.Number, Source: string(r.Source), Status: string(r.Status), Trigger: string(r.Trigger),
		GitRef: r.GitRef, CommitSHA: r.CommitSHA, Image: r.Image, Tag: r.Tag, Digest: r.Digest, Reference: r.Reference(),
		RunURL: r.RunURL, Failure: r.Failure, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

type requestDTO struct {
	Service string `json:"service" binding:"required"`
	Ref     string `json:"ref" binding:"required" description:"branch, tag or commit to build"`
}

type registerDTO struct {
	Service string `json:"service" binding:"required"`
	Tag     string `json:"tag" binding:"required" description:"an existing tag in the service's image repository"`
}

type listParams struct {
	Service string `query:"service" required:"true" description:"service slug"`
	apispec.ListParams
}

type releaseParams struct {
	ID string `path:"id"`
}

func (h ReleasesHandler) List(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Release], error) {
		return h.releases.PageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("service"), p)
	}
	apispec.RespondPage(c, load, ToDTO)
}

func (h ReleasesHandler) Get(c *gin.Context) {
	rel, err := h.releases.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	apispec.Respond(c, http.StatusOK, ToDTO(rel), err)
}

func (h ReleasesHandler) Request(c *gin.Context) {
	var body requestDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	rel, err := h.releases.Request(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Ref)
	apispec.Respond(c, http.StatusAccepted, ToDTO(rel), err)
}

func (h ReleasesHandler) Register(c *gin.Context) {
	var body registerDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	rel, err := h.releases.Register(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Tag)
	apispec.Respond(c, http.StatusCreated, ToDTO(rel), err)
}

func RegisterReleasesRoutes(api httpserver.API, h ReleasesHandler) {
	api.Handle(
		apispec.Operation{ID: "listReleases", Method: http.MethodGet, Path: "/releases", Summary: "List a service's releases, newest first",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[ReleaseDTO]{}, Handler: h.List},
		apispec.Operation{ID: "requestRelease", Method: http.MethodPost, Path: "/releases", Summary: "Build a git ref into a release; poll it until built or failed",
			Scope: scope.Builds, Body: requestDTO{}, Status: http.StatusAccepted, Response: ReleaseDTO{}, Handler: h.Request},
		apispec.Operation{ID: "registerRelease", Method: http.MethodPost, Path: "/releases/registrations", Summary: "Register an image tag that was built elsewhere",
			Scope: scope.Builds, Body: registerDTO{}, Status: http.StatusCreated, Response: ReleaseDTO{}, Handler: h.Register},
		apispec.Operation{ID: "getRelease", Method: http.MethodGet, Path: "/releases/:id", Summary: "Show a release",
			Scope: scope.Read, Params: releaseParams{}, Status: http.StatusOK, Response: ReleaseDTO{}, Handler: h.Get},
	)
}

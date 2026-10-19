package releases

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Releases interface {
	PageFor(ctx context.Context, a actor.Actor, serviceSlug string, p page.Request) (page.Page[entities.Release], error)
	GetFor(ctx context.Context, a actor.Actor, id string) (entities.Release, error)
	ResolveFor(ctx context.Context, a actor.Actor, serviceSlug, ref string) (entities.Release, error)
	Request(ctx context.Context, a actor.Actor, serviceSlug, ref string) (entities.Release, error)
	Register(ctx context.Context, a actor.Actor, serviceSlug, tag string) (entities.Release, error)
}

type Changelogs interface {
	Get(ctx context.Context, rel entities.Release) (entities.Changelog, error)
}

type ReleasesHandler struct {
	releases   Releases
	changelogs Changelogs
	waiter     wait.Waiter
}

func NewReleasesHandler(releases Releases, changelogs Changelogs, waiter wait.Waiter) ReleasesHandler {
	return ReleasesHandler{releases: releases, changelogs: changelogs, waiter: waiter}
}

type ReleaseDTO struct {
	ID         string        `json:"id"`
	Number     int           `json:"number"`
	Source     string        `json:"source" enum:"build,registered,shared"`
	Status     string        `json:"status" enum:"requested,building,built,failed"`
	Trigger    string        `json:"trigger" enum:"manual,push,register"`
	GitRef     string        `json:"git_ref,omitempty"`
	CommitSHA  string        `json:"commit_sha,omitempty"`
	Image      string        `json:"image"`
	Tag        string        `json:"tag,omitempty"`
	Digest     string        `json:"digest,omitempty"`
	Reference  string        `json:"reference,omitempty"`
	RunURL     string        `json:"run_url,omitempty"`
	Failure    string        `json:"failure,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	Changelog  *ChangelogDTO `json:"changelog,omitempty"`
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
	ID      string `path:"id" description:"a release id, or with service: latest, r42, sha:<7 to 40 hex> or a tag"`
	Service string `query:"service" description:"the service slug that id is resolved in"`
}

func (h ReleasesHandler) List(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Release], error) {
		return h.releases.PageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("service"), p)
	}
	apispec.RespondPage(c, load, ToDTO)
}

func (h ReleasesHandler) Get(c *gin.Context) {
	d, ok := wait.Duration(c)
	if !ok {
		return
	}
	id, err := h.releaseID(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	wait.Read(c, h.waiter, d, h.rereadWithChangelog(c, id), finalRelease)
}

func (h ReleasesHandler) releaseID(c *gin.Context) (string, error) {
	service := c.Query("service")
	if service == "" {
		return c.Param("id"), nil
	}
	rel, err := h.releases.ResolveFor(c.Request.Context(), requestctx.ActorFrom(c), service, c.Param("id"))
	return rel.ID, err
}

func (h ReleasesHandler) Request(c *gin.Context) {
	d, ok := wait.Duration(c)
	var body requestDTO
	if !ok || !httperror.BindStrict(c, &body) {
		return
	}
	rel, err := h.releases.Request(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Ref)
	wait.Started(c, h.waiter, d, ToDTO(rel), err, h.reread(c, rel.ID), finalRelease)
}

func (h ReleasesHandler) reread(c *gin.Context, id string) wait.Load[ReleaseDTO] {
	a := requestctx.ActorFrom(c)
	return func(ctx context.Context) (ReleaseDTO, error) {
		rel, err := h.releases.GetFor(ctx, a, id)
		return ToDTO(rel), err
	}
}

func (h ReleasesHandler) rereadWithChangelog(c *gin.Context, id string) wait.Load[ReleaseDTO] {
	a := requestctx.ActorFrom(c)
	return func(ctx context.Context) (ReleaseDTO, error) {
		rel, err := h.releases.GetFor(ctx, a, id)
		if err != nil {
			return ReleaseDTO{}, err
		}
		log, err := h.changelogs.Get(ctx, rel)
		dto := ToDTO(rel)
		dto.Changelog = toChangelogDTO(log)
		return dto, err
	}
}

func finalRelease(dto ReleaseDTO) bool {
	return entities.Release{Status: entities.Status(dto.Status)}.Final()
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
			Scope: scope.Builds, Waits: true, Body: requestDTO{}, Status: http.StatusAccepted, Response: ReleaseDTO{}, Handler: h.Request},
		apispec.Operation{ID: "registerRelease", Method: http.MethodPost, Path: "/releases/registrations", Summary: "Register an image tag that was built elsewhere",
			Scope: scope.Builds, Body: registerDTO{}, Status: http.StatusCreated, Response: ReleaseDTO{}, Handler: h.Register},
		apispec.Operation{ID: "getRelease", Method: http.MethodGet, Path: "/releases/:id", Summary: "Show a release by id, or by latest, r42, sha:<prefix> or tag within ?service",
			Scope: scope.Read, Waits: true, Params: releaseParams{}, Status: http.StatusOK, Response: ReleaseDTO{}, Handler: h.Get},
	)
}

package releases

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Releases interface {
	ListFor(ctx context.Context, a actor.Actor, serviceSlug string, limit int) ([]entities.Release, error)
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
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	Trigger    string     `json:"trigger"`
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
	Ref     string `json:"ref" binding:"required"`
}

type registerDTO struct {
	Service string `json:"service" binding:"required"`
	Tag     string `json:"tag" binding:"required"`
}

func (h ReleasesHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	list, err := h.releases.ListFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("service"), limit)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]ReleaseDTO, 0, len(list))
	for _, r := range list {
		result = append(result, ToDTO(r))
	}
	c.JSON(http.StatusOK, gin.H{"releases": result})
}

func (h ReleasesHandler) Get(c *gin.Context) {
	rel, err := h.releases.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	respond(c, http.StatusOK, rel, err)
}

func (h ReleasesHandler) Request(c *gin.Context) {
	var body requestDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	rel, err := h.releases.Request(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Ref)
	respond(c, http.StatusAccepted, rel, err)
}

func (h ReleasesHandler) Register(c *gin.Context) {
	var body registerDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	rel, err := h.releases.Register(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Tag)
	respond(c, http.StatusCreated, rel, err)
}

func respond(c *gin.Context, status int, rel entities.Release, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(status, ToDTO(rel))
}

func RegisterReleasesRoutes(api httpserver.API, h ReleasesHandler) {
	api.GET("/releases", h.List)
	api.POST("/releases", h.Request)
	api.POST("/releases/registrations", h.Register)
	api.GET("/releases/:id", h.Get)
}

package callbacks

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/handlers/releases"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/oidc"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const callerKey = "platform.build_caller"

type TokenVerifier interface {
	Verify(ctx context.Context, raw string) (oidc.Claims, error)
}

type Builds interface {
	Started(ctx context.Context, caller entities.BuildCaller, releaseID, runURL string) (entities.Release, error)
	Built(ctx context.Context, caller entities.BuildCaller, releaseID string, report entities.BuildReport) (entities.Release, error)
	Failed(ctx context.Context, caller entities.BuildCaller, releaseID string, report entities.BuildReport) (entities.Release, error)
	RegistryCredentials(ctx context.Context, caller entities.BuildCaller, releaseID string) (entities.RegistryCredentials, error)
}

var errNoToken = apperror.New(apperror.CodeUnauthenticated, "send a GitHub Actions OIDC token as a bearer token")

type CallbacksHandler struct {
	verifier TokenVerifier
	builds   Builds
}

func NewCallbacksHandler(verifier TokenVerifier, builds Builds) CallbacksHandler {
	return CallbacksHandler{verifier: verifier, builds: builds}
}

type reportDTO struct {
	Digest    string `json:"digest"`
	CommitSHA string `json:"commit_sha"`
	Tag       string `json:"tag"`
	RunURL    string `json:"run_url"`
	Failure   string `json:"failure"`
}

func (d reportDTO) toEntity() entities.BuildReport {
	return entities.BuildReport(d)
}

func (h CallbacksHandler) authenticate(c *gin.Context) {
	raw, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !found || raw == "" {
		httperror.Abort(c, errNoToken)
		return
	}
	claims, err := h.verifier.Verify(c.Request.Context(), raw)
	if err != nil {
		httperror.Abort(c, apperror.New(apperror.CodeUnauthenticated, "invalid OIDC token: "+err.Error()))
		return
	}
	c.Set(callerKey, entities.BuildCaller{Repository: claims.Repository, WorkflowRef: claims.JobWorkflowRef})
}

func (h CallbacksHandler) Started(c *gin.Context) {
	body, ok := bind(c)
	if !ok {
		return
	}
	rel, err := h.builds.Started(c.Request.Context(), callerFrom(c), c.Param("id"), body.RunURL)
	respond(c, rel, err)
}

func (h CallbacksHandler) Built(c *gin.Context) {
	body, ok := bind(c)
	if !ok {
		return
	}
	rel, err := h.builds.Built(c.Request.Context(), callerFrom(c), c.Param("id"), body.toEntity())
	respond(c, rel, err)
}

func (h CallbacksHandler) Failed(c *gin.Context) {
	body, ok := bind(c)
	if !ok {
		return
	}
	rel, err := h.builds.Failed(c.Request.Context(), callerFrom(c), c.Param("id"), body.toEntity())
	respond(c, rel, err)
}

func bind(c *gin.Context) (reportDTO, bool) {
	var body reportDTO
	return body, httperror.BindJSON(c, &body)
}

func callerFrom(c *gin.Context) entities.BuildCaller {
	caller, _ := c.Get(callerKey)
	result, _ := caller.(entities.BuildCaller)
	return result
}

func respond(c *gin.Context, rel entities.Release, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, releases.ToDTO(rel))
}

func RegisterCallbacksRoutes(public httpserver.Public, h CallbacksHandler) {
	group := public.Group("/api/v1/releases/:id", h.authenticate)
	group.POST("/started", h.Started)
	group.POST("/built", h.Built)
	group.POST("/failed", h.Failed)
	group.POST("/registry-credentials", h.RegistryCredentials)
}

type credentialsDTO struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h CallbacksHandler) RegistryCredentials(c *gin.Context) {
	creds, err := h.builds.RegistryCredentials(c.Request.Context(), callerFrom(c), c.Param("id"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, credentialsDTO(creds))
}

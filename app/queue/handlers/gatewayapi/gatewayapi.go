package gatewayapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/gateway"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

var (
	errNotConfigured = apperror.New(apperror.CodeNotConfigured, "the queue gateway is not configured on this control plane")
	errBadToken      = apperror.New(apperror.CodeUnauthenticated, "invalid queue gateway token")
)

type Gateway interface {
	Desired(ctx context.Context) (gateway.Desired, error)
	Report(ctx context.Context, stats []entities.Stats) error
	Done(ctx context.Context, commandID string) error
}

type GatewayAPIHandler struct {
	token   []byte
	gateway Gateway
}

func NewGatewayAPIHandler(cfg config.Config, gateway Gateway) GatewayAPIHandler {
	return GatewayAPIHandler{token: []byte(cfg.Queues.Token), gateway: gateway}
}

func (h GatewayAPIHandler) authenticate(c *gin.Context) {
	if len(h.token) == 0 {
		httperror.Abort(c, errNotConfigured)
		return
	}
	sent, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(sent), h.token) != 1 {
		httperror.Abort(c, errBadToken)
	}
}

func (h GatewayAPIHandler) Desired(c *gin.Context) {
	desired, err := h.gateway.Desired(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDesiredDTO(desired))
}

func (h GatewayAPIHandler) Stats(c *gin.Context) {
	var body queuedapi.StatsReport
	if !httperror.BindJSON(c, &body) {
		return
	}
	reply(c, h.gateway.Report(c.Request.Context(), toEntityStats(body)))
}

func (h GatewayAPIHandler) Done(c *gin.Context) {
	reply(c, h.gateway.Done(c.Request.Context(), c.Param("id")))
}

func reply(c *gin.Context, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterGatewayAPIRoutes(public httpserver.Public, h GatewayAPIHandler) {
	public.GET(queuedapi.DesiredPath, h.authenticate, h.Desired)
	public.POST(queuedapi.StatsPath, h.authenticate, h.Stats)
	public.POST(queuedapi.DonePath(":id"), h.authenticate, h.Done)
}

package gatewayapi

import (
	"context"
	"crypto/subtle"
	"errors"
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

type Crons interface {
	Desired(ctx context.Context) ([]queuedapi.Cron, []queuedapi.Command, error)
	Record(ctx context.Context, runs []queuedapi.CronRun) error
	Done(ctx context.Context, commandID string) error
}

type GatewayAPIHandler struct {
	token   []byte
	gateway Gateway
	crons   Crons
}

func NewGatewayAPIHandler(cfg config.Config, gateway Gateway, crons Crons) GatewayAPIHandler {
	return GatewayAPIHandler{token: []byte(cfg.Queues.Token), gateway: gateway, crons: crons}
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
	desired, err := h.desired(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, desired)
}

func (h GatewayAPIHandler) desired(ctx context.Context) (queuedapi.Desired, error) {
	queues, err := h.gateway.Desired(ctx)
	if err != nil {
		return queuedapi.Desired{}, err
	}
	crons, commands, err := h.crons.Desired(ctx)
	desired := toDesiredDTO(queues)
	desired.Crons, desired.Commands = crons, append(desired.Commands, commands...)
	return desired, err
}

func (h GatewayAPIHandler) CronRuns(c *gin.Context) {
	var body queuedapi.CronRunReport
	if !httperror.BindJSON(c, &body) {
		return
	}
	reply(c, h.crons.Record(c.Request.Context(), body.Runs))
}

func (h GatewayAPIHandler) Stats(c *gin.Context) {
	var body queuedapi.StatsReport
	if !httperror.BindJSON(c, &body) {
		return
	}
	reply(c, h.gateway.Report(c.Request.Context(), toEntityStats(body)))
}

func (h GatewayAPIHandler) Done(c *gin.Context) {
	ctx, id := c.Request.Context(), c.Param("id")
	reply(c, errors.Join(h.gateway.Done(ctx, id), h.crons.Done(ctx, id)))
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
	public.POST(queuedapi.CronRunsPath, h.authenticate, h.CronRuns)
	public.POST(queuedapi.DonePath(":id"), h.authenticate, h.Done)
}

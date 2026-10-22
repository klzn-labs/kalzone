package telemetry

import (
	"context"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	telemetryUsecase "github.com/klzn-labs/kalzone/app/observe/usecases/telemetry"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	Prefix       = "/telemetry"
	requestLimit = 1 << 20
)

type Telemetry interface {
	Prometheus(ctx context.Context, r telemetryUsecase.PrometheusRequest) (entities.TelemetryAnswer, error)
	Loki(ctx context.Context, r telemetryUsecase.LokiRequest) (entities.TelemetryAnswer, error)
}

type TelemetryHandler struct {
	telemetry Telemetry
}

func NewTelemetryHandler(telemetry Telemetry) TelemetryHandler {
	return TelemetryHandler{telemetry: telemetry}
}

func RegisterTelemetryRoutes(public httpserver.Public, h TelemetryHandler) {
	public.Any(Prefix+"/:team/prometheus/*path", h.Prometheus)
	public.Any(Prefix+"/:team/loki/*path", h.Loki)
}

func (h TelemetryHandler) Prometheus(c *gin.Context) {
	c.Request.Body = io.NopCloser(io.LimitReader(c.Request.Body, requestLimit))
	if err := c.Request.ParseForm(); err != nil {
		httperror.Abort(c, apperror.Invalid("the form could not be read"))
		return
	}
	r := telemetryUsecase.PrometheusRequest{Team: c.Param("team"), Token: bearer(c), Path: c.Param("path"), Form: c.Request.Form}
	answer, err := h.telemetry.Prometheus(c.Request.Context(), r)
	respond(c, answer, err)
}

func (h TelemetryHandler) Loki(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, requestLimit))
	if err != nil {
		httperror.Abort(c, apperror.Invalid("the body could not be read"))
		return
	}
	call := entities.LokiCall{
		Method: c.Request.Method, Path: c.Param("path"), RawQuery: c.Request.URL.RawQuery,
		ContentType: c.GetHeader("Content-Type"), Body: body,
	}
	answer, err := h.telemetry.Loki(c.Request.Context(), telemetryUsecase.LokiRequest{Team: c.Param("team"), Token: bearer(c), Call: call})
	respond(c, answer, err)
}

func bearer(c *gin.Context) string {
	return strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
}

func respond(c *gin.Context, answer entities.TelemetryAnswer, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Data(answer.Status, answer.ContentType, answer.Body)
}

package logs

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/observe/entities"
	logsUsecase "github.com/adrianozp/platform/app/observe/usecases/logs"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Logs interface {
	Read(ctx context.Context, a actor.Actor, serviceSlug, environment string, q logsUsecase.Query) ([]entities.LogLine, error)
}

type logsParams struct {
	Service  string `path:"service" description:"service slug"`
	Env      string `path:"env" description:"environment slug"`
	Since    string `query:"since" description:"how far back, as a duration above 0 and at most 24h; 15m by default"`
	Limit    int    `query:"limit" minimum:"1" maximum:"1000" default:"100" description:"the newest N lines in the window"`
	Contains string `query:"contains" description:"only lines containing this text"`
	Colour   string `query:"colour" enum:"blue,green" description:"only this colour; both by default"`
}

type lineDTO struct {
	At        time.Time `json:"at"`
	Container string    `json:"container"`
	Line      string    `json:"line"`
}

type LogsHandler struct {
	logs Logs
}

func NewLogsHandler(logs Logs) LogsHandler {
	return LogsHandler{logs: logs}
}

func RegisterLogsRoutes(api httpserver.API, h LogsHandler) {
	api.Handle(apispec.Operation{
		ID: "listLogs", Method: http.MethodGet, Path: "/logs/:service/:env",
		Summary: "Read a service's recent container logs in one environment, oldest first; needs the read and logs scopes",
		Scope:   scope.Logs, Params: logsParams{}, Status: http.StatusOK, Response: apispec.Page[lineDTO]{}, Handler: h.List,
	})
}

func (h LogsHandler) List(c *gin.Context) {
	q, err := logsUsecase.ParseQuery(c.Query("since"), c.Query("limit"), c.Query("contains"), c.Query("colour"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	lines, err := h.logs.Read(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), q)
	apispec.Respond(c, http.StatusOK, toPage(lines), err)
}

func toPage(lines []entities.LogLine) apispec.Page[lineDTO] {
	items := make([]lineDTO, 0, len(lines))
	for _, l := range lines {
		items = append(items, lineDTO{At: l.At, Container: l.Container, Line: l.Line})
	}
	return apispec.Page[lineDTO]{Items: items}
}

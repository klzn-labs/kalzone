package search

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/search"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Search interface {
	Find(ctx context.Context, a actor.Actor, q search.Query) ([]entities.Match, error)
}

type SearchHandler struct {
	search Search
}

func NewSearchHandler(search Search) SearchHandler {
	return SearchHandler{search: search}
}

type matchDTO struct {
	Service     string  `json:"service"`
	Environment string  `json:"environment"`
	Revision    int     `json:"revision"`
	Kind        string  `json:"kind" enum:"env,secret"`
	Value       *string `json:"value,omitempty" description:"the value of an env variable; never sent for a secret"`
}

func toMatchDTO(m entities.Match) matchDTO {
	dto := matchDTO{Service: m.ServiceSlug, Environment: m.Environment, Revision: m.Revision, Kind: string(m.Kind)}
	if m.Kind == entities.MatchEnv {
		dto.Value = &m.Value
	}
	return dto
}

func matchKey(m entities.Match) string {
	return m.ServiceSlug + "/" + m.Environment
}

type searchParams struct {
	Key         string `query:"key" required:"true" description:"the exact variable or secret name, such as LOG_LEVEL"`
	Environment string `query:"env" description:"only this environment"`
	Team        string `query:"team" description:"only services of this team"`
	apispec.ListParams
}

func (h SearchHandler) Find(c *gin.Context) {
	q := search.Query{Key: c.Query("key"), Environment: c.Query("env"), Team: c.Query("team")}
	load := func() ([]entities.Match, error) {
		return h.search.Find(c.Request.Context(), requestctx.ActorFrom(c), q)
	}
	apispec.RespondWindow(c, load, matchKey, toMatchDTO)
}

func RegisterSearchRoutes(api httpserver.API, h SearchHandler) {
	api.Handle(apispec.Operation{
		ID: "searchConfig", Method: http.MethodGet, Path: "/config/search", Summary: "Find where a variable or secret is set in current configs; secrets by name only",
		Scope: scope.Read, Params: searchParams{}, Status: http.StatusOK, Response: apispec.Page[matchDTO]{}, Handler: h.Find,
	})
}

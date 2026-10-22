package teams

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Teams interface {
	ListFor(ctx context.Context, a actor.Actor) ([]entities.Team, error)
	GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Team, error)
	Create(ctx context.Context, a actor.Actor, slug, name string) (entities.Team, error)
	Delete(ctx context.Context, a actor.Actor, slug string) error
}

type TeamsHandler struct {
	teams Teams
}

func NewTeamsHandler(teams Teams) TeamsHandler {
	return TeamsHandler{teams: teams}
}

type TeamDTO struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func ToDTO(t entities.Team) TeamDTO {
	return TeamDTO{ID: t.ID, Slug: t.Slug, Name: t.Name, CreatedAt: t.CreatedAt}
}

type createDTO struct {
	Slug string `json:"slug" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type teamParams struct {
	Team string `path:"team"`
}

func (h TeamsHandler) List(c *gin.Context) {
	apispec.RespondWindow(c, func() ([]entities.Team, error) { return h.teams.ListFor(c.Request.Context(), requestctx.ActorFrom(c)) }, teamKey, ToDTO)
}

func teamKey(t entities.Team) string {
	return t.Slug
}

func (h TeamsHandler) Get(c *gin.Context) {
	team, err := h.teams.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"))
	apispec.Respond(c, http.StatusOK, ToDTO(team), err)
}

func (h TeamsHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	team, err := h.teams.Create(c.Request.Context(), requestctx.ActorFrom(c), body.Slug, body.Name)
	apispec.Respond(c, http.StatusCreated, ToDTO(team), err)
}

func (h TeamsHandler) Delete(c *gin.Context) {
	if err := h.teams.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterTeamsRoutes(api httpserver.API, h TeamsHandler) {
	api.Handle(
		apispec.Operation{ID: "listTeams", Method: http.MethodGet, Path: "/teams", Summary: "List teams by slug",
			Scope: scope.Read, Params: apispec.ListParams{}, Status: http.StatusOK, Response: apispec.Page[TeamDTO]{}, Handler: h.List},
		apispec.Operation{ID: "createTeam", Method: http.MethodPost, Path: "/teams", Summary: "Create a team",
			Scope: scope.TeamsAdmin, Body: createDTO{}, Status: http.StatusCreated, Response: TeamDTO{}, Handler: h.Create},
		apispec.Operation{ID: "getTeam", Method: http.MethodGet, Path: "/teams/:team", Summary: "Show a team",
			Scope: scope.Read, Params: teamParams{}, Status: http.StatusOK, Response: TeamDTO{}, Handler: h.Get},
		apispec.Operation{ID: "deleteTeam", Method: http.MethodDelete, Path: "/teams/:team", Summary: "Delete a team that owns no services",
			Scope: scope.TeamsAdmin, Params: teamParams{}, Status: http.StatusNoContent, Handler: h.Delete},
	)
}

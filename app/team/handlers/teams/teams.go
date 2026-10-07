package teams

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Teams interface {
	List(ctx context.Context) ([]entities.Team, error)
	Get(ctx context.Context, slug string) (entities.Team, error)
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

func (h TeamsHandler) List(c *gin.Context) {
	list, err := h.teams.List(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]TeamDTO, 0, len(list))
	for _, t := range list {
		result = append(result, ToDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{"teams": result})
}

func (h TeamsHandler) Get(c *gin.Context) {
	team, err := h.teams.Get(c.Request.Context(), c.Param("team"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, ToDTO(team))
}

func (h TeamsHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	team, err := h.teams.Create(c.Request.Context(), requestctx.ActorFrom(c), body.Slug, body.Name)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, ToDTO(team))
}

func (h TeamsHandler) Delete(c *gin.Context) {
	if err := h.teams.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterTeamsRoutes(api httpserver.API, h TeamsHandler) {
	api.GET("/teams", h.List)
	api.POST("/teams", h.Create)
	api.GET("/teams/:team", h.Get)
	api.DELETE("/teams/:team", h.Delete)
}

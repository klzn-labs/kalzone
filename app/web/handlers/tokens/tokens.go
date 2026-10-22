package tokens

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Tokens interface {
	List(ctx context.Context, a actor.Actor) ([]identity.AccessToken, error)
	ListAll(ctx context.Context, a actor.Actor) ([]identity.AccessToken, error)
	Create(ctx context.Context, a actor.Actor, d identity.TokenDraft) (identity.IssuedToken, error)
	Revoke(ctx context.Context, a actor.Actor, id string) error
}

type AllTeams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Row struct {
	Token    identity.AccessToken
	Owner    string
	Teams    []string
	Status   string
	LastUsed time.Time
}

type data struct {
	Mine       []Row
	Everyone   []Row
	Teams      []team.Team
	Scopes     []scope.Scope
	IsAdmin    bool
	Issued     string
	IssuedName string
}

type TokensHandler struct {
	renderer *web.Renderer
	views    web.Views
	tokens   Tokens
	teams    AllTeams
}

func NewTokensHandler(renderer *web.Renderer, views web.Views, tokens Tokens, teams AllTeams) TokensHandler {
	return TokensHandler{renderer: renderer, views: views, tokens: tokens, teams: teams}
}

func (h TokensHandler) Show(c *gin.Context) {
	if requestctx.ActorFrom(c).Guest {
		web.ErrorPage(c, h.renderer, web.ErrNotInDemo)
		return
	}
	h.render(c, identity.IssuedToken{})
}

func (h TokensHandler) Create(c *gin.Context) {
	issued, err := h.tokens.Create(c.Request.Context(), requestctx.ActorFrom(c), draftFrom(c))
	if err != nil {
		web.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	h.render(c, issued)
}

func (h TokensHandler) Revoke(c *gin.Context) {
	if err := h.tokens.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id")); err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/tokens", "Token revoked")
}

func draftFrom(c *gin.Context) identity.TokenDraft {
	days, _ := strconv.Atoi(c.PostForm("expires_in_days"))
	return identity.TokenDraft{
		Name: strings.TrimSpace(c.PostForm("name")), Scopes: c.PostFormArray("scopes"), Teams: c.PostFormArray("teams"),
		Bypass: c.PostForm("bypass") == "on", ExpiresInDays: days,
	}
}

func (h TokensHandler) render(c *gin.Context, issued identity.IssuedToken) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d.Issued, d.IssuedName = issued.Secret, issued.Token.Name
	h.renderer.Render(c, http.StatusOK, "tokens", web.Page{Title: "Access tokens", Nav: "tokens", Data: d})
}

func (h TokensHandler) build(ctx context.Context, a actor.Actor) (data, error) {
	mine, err := h.tokens.List(ctx, a)
	if err != nil {
		return data{}, err
	}
	d := data{Scopes: scope.All, IsAdmin: a.Admin}
	if d.Teams, err = h.teamChoices(ctx, a); err != nil {
		return data{}, err
	}
	if d.Mine, err = h.rows(ctx, mine); err != nil || !a.Admin {
		return d, err
	}
	everyone, err := h.tokens.ListAll(ctx, a)
	if err != nil {
		return data{}, err
	}
	d.Everyone, err = h.rows(ctx, everyone)
	return d, err
}

func (h TokensHandler) teamChoices(ctx context.Context, a actor.Actor) ([]team.Team, error) {
	if a.Admin {
		return h.teams.List(ctx)
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	teams := make([]team.Team, 0, len(mine))
	for _, m := range mine {
		teams = append(teams, m.Team)
	}
	return teams, nil
}

func (h TokensHandler) rows(ctx context.Context, list []identity.AccessToken) ([]Row, error) {
	owners, err := h.views.UsersByID(ctx, ownerIDs(list))
	if err != nil {
		return nil, err
	}
	teams, err := h.views.TeamsByID(ctx, teamIDs(list))
	if err != nil {
		return nil, err
	}
	now := clock.Now()
	result := make([]Row, 0, len(list))
	for _, t := range list {
		result = append(result, rowOf(t, owners[t.OwnerID].Login, teams, now))
	}
	return result, nil
}

func rowOf(t identity.AccessToken, owner string, teams map[string]team.Team, now time.Time) Row {
	row := Row{Token: t, Owner: owner, Teams: slugsOf(teams, t.TeamIDs), Status: statusOf(t, now)}
	if t.LastUsedAt != nil {
		row.LastUsed = *t.LastUsedAt
	}
	return row
}

func ownerIDs(list []identity.AccessToken) []string {
	ids := make([]string, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.OwnerID)
	}
	return ids
}

func teamIDs(list []identity.AccessToken) []string {
	ids := []string{}
	for _, t := range list {
		ids = append(ids, t.TeamIDs...)
	}
	return ids
}

func slugsOf(teams map[string]team.Team, ids []string) []string {
	slugs := make([]string, 0, len(ids))
	for _, id := range ids {
		slugs = append(slugs, teams[id].Slug)
	}
	return slugs
}

func statusOf(t identity.AccessToken, now time.Time) string {
	switch {
	case t.RevokedAt != nil:
		return "revoked"
	case !now.Before(t.ExpiresAt):
		return "expired"
	}
	return "active"
}

func RegisterTokensRoutes(routes httpserver.Web, h TokensHandler) {
	routes.GET("/tokens", h.Show)
	routes.POST("/tokens", h.Create)
	routes.POST("/tokens/:id/revoke", h.Revoke)
}
